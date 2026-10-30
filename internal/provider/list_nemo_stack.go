package provider

import (
	"context"
	"errors"
	"maps"
	"os"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/moby/moby/client"

	"github.com/sntns/nemo/pkg/runtime"
	"github.com/sntns/nemo/pkg/storage/volume"
	"github.com/sntns/nemo/pkg/target"
)

var (
	_ list.ListResource              = &nemoStackListResource{}
	_ list.ListResourceWithConfigure = &nemoStackListResource{}
)

// nemoStackIdentityModel is a stack's identity: its name, and the host of
// its target when it gives one, as an import ID gives them.
type nemoStackIdentityModel struct {
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

func (r *nemoStackResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"name": identityschema.StringAttribute{Description: "The stack's name.", RequiredForImport: true},
			"host": identityschema.StringAttribute{
				Description:       "The address of the target's Docker, when it is not the provider's nor the environment's.",
				OptionalForImport: true,
			},
		},
	}
}

// setIdentity sets the identity of the stack, from its name and its
// target's own host: one the provider gives is not the stack's, and can
// change without the stack changing.
func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, model nemoStackResourceModel, diags *diag.Diagnostics) {
	if identity == nil {
		return
	}
	host := types.StringNull()
	if model.Target != nil && !model.Target.Host.IsUnknown() {
		host = model.Target.Host
	}
	diags.Append(identity.Set(ctx, nemoStackIdentityModel{Name: model.Name, Host: host})...)
}

type nemoStackListResource struct {
	connection *nemoConnection
}

type nemoStackListModel struct {
	Host    types.String `tfsdk:"host"`
	Context types.String `tfsdk:"context"`
}

func NewNemoStackListResource() list.ListResource {
	return &nemoStackListResource{}
}

func (r *nemoStackListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (r *nemoStackListResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if connection, ok := req.ProviderData.(*nemoConnection); ok {
		r.connection = connection
	}
}

func (r *nemoStackListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		MarkdownDescription: "The stacks a Docker has the records of, for `terraform query`: each, with its installations, ready to import.",
		Attributes: map[string]listschema.Attribute{
			"host": listschema.StringAttribute{
				MarkdownDescription: "The address of the Docker: `tcp://`, `ssh://user@host` or `unix://`. " +
					"With neither `host` nor `context`, the provider's, with its `tls`; else the environment's.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("context"))},
			},
			"context": listschema.StringAttribute{MarkdownDescription: "A docker context to reach the Docker with, instead of `host`.", Optional: true},
		},
	}
}

func (r *nemoStackListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config nemoStackListModel
	var diags diag.Diagnostics
	diags.Append(req.Config.Get(ctx, &config)...)
	given := nemoStackResourceModel{Target: &nemoStackTargetModel{Host: config.Host, Context: config.Context}}
	directory, err := os.Getwd()
	if err != nil {
		diags.AddError("nemo stack paths failed", err.Error())
	}
	reached := expandTarget(ctx, r.connection.over(given).Target, directory, &diags)
	var names []string
	if !diags.HasError() {
		if names, err = recordedStacks(ctx, reached); err != nil {
			diags.AddError("nemo stack list failed", err.Error())
		}
	}
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		listed := int64(0)
		for _, name := range names {
			if req.Limit > 0 && listed == req.Limit {
				return
			}
			model := nemoStackResourceModel{
				ID: types.StringValue(name), Name: types.StringValue(name),
				Target: &nemoStackTargetModel{Host: config.Host, Context: config.Context},
			}
			result, found := r.listed(ctx, req, model)
			if !found {
				continue
			}
			listed++
			if !push(result) {
				return
			}
		}
	}
}

// listed is the stack as importing it gives it, when its target has
// installations of it.
func (r *nemoStackListResource) listed(ctx context.Context, req list.ListRequest, model nemoStackResourceModel) (list.ListResult, bool) {
	result := req.NewListResult(ctx)
	result.DisplayName = model.Name.ValueString()
	present, err := installedOn(ctx, expandStack(ctx, r.connection.over(model), &result.Diagnostics))
	if err != nil {
		result.Diagnostics.AddError("nemo stack read failed", err.Error())
		return result, true
	}
	if len(present) == 0 {
		return result, false
	}
	refreshInstallations(ctx, &model, present, &result.Diagnostics)
	setIdentity(ctx, result.Identity, model, &result.Diagnostics)
	if !req.IncludeResource {
		return result, true
	}

	hash, err := contentHash(installationFiles(model.Installations))
	if err != nil {
		result.Diagnostics.AddError("nemo stack read failed", err.Error())
	}
	model.ContentHash = hash
	model.Manifest = encodeManifest(expandStack(ctx, r.connection.over(model), &result.Diagnostics), &result.Diagnostics)
	// Attribute by attribute: those left out, such as timeouts, stay null.
	for name, value := range map[string]any{
		"id": model.ID, "name": model.Name, "target": model.Target, "installer": model.Installer, "installations": model.Installations,
		"purge_on_destroy": model.PurgeOnDestroy, "content_hash": model.ContentHash, "manifest": model.Manifest,
	} {
		result.Diagnostics.Append(result.Resource.SetAttribute(ctx, path.Root(name), value)...)
	}
	return result, true
}

// recordedStacks are the names of the stacks whose records the Docker has,
// from the label nemo gives their volumes.
func recordedStacks(ctx context.Context, reached target.Target) (_ []string, err error) {
	api, err := runtime.Connect(reached)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, api.Close()) }()
	volumes, err := api.VolumeList(ctx, client.VolumeListOptions{Filters: make(client.Filters).Add("label", volume.Label)})
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, recorded := range volumes.Items {
		if name := recorded.Labels[volume.Label]; name != "" {
			names[name] = true
		}
	}
	return slices.Sorted(maps.Keys(names)), nil
}
