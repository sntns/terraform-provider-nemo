package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sntns/nemo/pkg/target"
)

var (
	_ resource.Resource                = &nemoStackResource{}
	_ resource.ResourceWithImportState = &nemoStackResource{}
	_ resource.ResourceWithModifyPlan  = &nemoStackResource{}
	_ resource.ResourceWithConfigure   = &nemoStackResource{}
)

// dockerName is what Docker accepts in the names nemo gives after the stack.
var dockerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type nemoStackResource struct {
	connection *nemoConnection
}

type nemoStackResourceModel struct {
	ID             types.String                    `tfsdk:"id"`
	Name           types.String                    `tfsdk:"name"`
	Target         *nemoStackTargetModel           `tfsdk:"target"`
	Installer      *nemoStackInstallerModel        `tfsdk:"installer"`
	Remote         types.String                    `tfsdk:"remote"`
	Parameters     types.Map                       `tfsdk:"parameters"`
	Credentials    map[string]nemoStackSourceModel `tfsdk:"credentials"`
	Installations  []nemoStackInstallationModel    `tfsdk:"installations"`
	PurgeOnDestroy types.Bool                      `tfsdk:"purge_on_destroy"`
	Timeouts       timeouts.Value                  `tfsdk:"timeouts"`
	ContentHash    types.String                    `tfsdk:"content_hash"`
	Manifest       types.String                    `tfsdk:"manifest"`
}

type nemoStackTargetModel struct {
	Orchestrator types.String                        `tfsdk:"orchestrator"`
	Host         types.String                        `tfsdk:"host"`
	Context      types.String                        `tfsdk:"context"`
	TLS          *nemoStackTLSModel                  `tfsdk:"tls"`
	Images       types.String                        `tfsdk:"images"`
	Volumes      map[string]nemoStackVolumeTypeModel `tfsdk:"volumes"`
}

type nemoStackTLSModel struct {
	CA   *nemoStackTLSSourceModel `tfsdk:"ca"`
	Cert *nemoStackTLSSourceModel `tfsdk:"cert"`
	Key  *nemoStackTLSSourceModel `tfsdk:"key"`
}

// nemoStackTLSSourceModel is where a part of a TLS is read. Every operation
// reaches the target with it, reading and destroying included, which have
// the state alone: it is never write-only.
type nemoStackTLSSourceModel struct {
	Path    types.String `tfsdk:"path"`
	Env     types.String `tfsdk:"env"`
	Command types.String `tfsdk:"command"`
	Value   types.String `tfsdk:"value"`
}

type nemoStackVolumeTypeModel struct {
	Driver    types.String                   `tfsdk:"driver"`
	Options   types.Map                      `tfsdk:"options"`
	Provision *nemoStackVolumeProvisionModel `tfsdk:"provision"`
}

type nemoStackVolumeProvisionModel struct {
	Directory types.String               `tfsdk:"directory"`
	Volume    nemoStackVolumeDriverModel `tfsdk:"volume"`
}

type nemoStackVolumeDriverModel struct {
	Driver  types.String `tfsdk:"driver"`
	Options types.Map    `tfsdk:"options"`
}

type nemoStackInstallerModel struct {
	Driver     types.String              `tfsdk:"driver"`
	Kubernetes *nemoStackKubernetesModel `tfsdk:"kubernetes"`
}

type nemoStackKubernetesModel struct {
	Namespace  types.String `tfsdk:"namespace"`
	Kubeconfig types.String `tfsdk:"kubeconfig"`
}

type nemoStackInstallationModel struct {
	Name        types.String                    `tfsdk:"name"`
	Version     types.String                    `tfsdk:"version"`
	Repository  types.String                    `tfsdk:"repository"`
	File        types.String                    `tfsdk:"file"`
	Parameters  types.Map                       `tfsdk:"parameters"`
	Credentials map[string]nemoStackSourceModel `tfsdk:"credentials"`
}

// nemoStackSourceModel is where a credential, or a part of a TLS, is read:
// one of a file, a variable, a command or a value, written-only or not.
type nemoStackSourceModel struct {
	Path           types.String `tfsdk:"path"`
	Env            types.String `tfsdk:"env"`
	Command        types.String `tfsdk:"command"`
	Value          types.String `tfsdk:"value"`
	ValueWO        types.String `tfsdk:"value_wo"`
	ValueWOVersion types.String `tfsdk:"value_wo_version"`
}

func NewNemoStackResource() resource.Resource {
	return &nemoStackResource{}
}

func (r *nemoStackResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (r *nemoStackResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if connection, ok := req.ProviderData.(*nemoConnection); ok {
		r.connection = connection
	}
}

func (r *nemoStackResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A stack: bundles deployed together on one target, as nemo deploys a manifest with `nemo up`. " +
			"Each apply brings the stack to its configuration: it installs the bundles it lists, upgrades those installed, " +
			"and uninstalls those it no longer lists. Reading it reads the records of its installations on the target, so that an installation " +
			"nemo changed with `--force` shows in the next plan. Destroying it uninstalls every installation. " +
			"A stack its target has already is imported, not created. " +
			"Planning loads the bundles it installs or upgrades, from their registry, layout or `bundle.json`: " +
			"one that is not there, or is another installation's, fails the plan rather than the apply.\n\n" +
			"The records of its installations are kept on its target (`records: target` in nemo's manifest): " +
			"nemo reads them from any machine that imported `manifest`, and refuses to write the stack without `--force`.\n\n" +
			"nemo bounds each action on an installation, by its `nemo-timeout` parameter: `5m` unless the stack's `parameters` give another. " +
			"`timeouts` bounds the whole operation on top of it; without it, Terraform waits as long as nemo does.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The stack's name.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The stack's name: Swarm names its services `<name>_<service>`, in 63 characters. Changing it replaces the stack.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.RegexMatches(dockerName, "must be letters, digits, _ . and - only")},
			},
			"target":    targetSchema(),
			"installer": installerSchema(),
			"remote": schema.StringAttribute{
				MarkdownDescription: "Where the bundles of the installations that give a version alone are: a template of " +
					"`{{ .Component }}`, the installation's name, and `{{ .Version }}`, its version, such as " +
					"`docker://ghcr.io/sntns/{{ .Component }}:{{ .Version }}`.",
				Optional: true,
			},
			"parameters": schema.MapAttribute{
				MarkdownDescription: "Parameters, by name, for every installation whose bundle declares them. An installation's own come over them.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"credentials": credentialsSchema("Credentials, by name, for every installation whose bundle declares them. An installation's own come over them."),
			"installations": schema.ListNestedAttribute{
				MarkdownDescription: "The installations of the stack, one per bundle, installed in the order of their dependencies.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: "The installation's name, its bundle's.",
						Required:            true,
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "The bundle's version: with `remote`, or with `repository`. Reading the stack sets the version deployed.",
						Optional:            true,
					},
					"repository": schema.StringAttribute{
						MarkdownDescription: "The bundle's repository in a registry, instead of `remote`: `ghcr.io/sntns/whoami`.",
						Optional:            true,
					},
					"file": schema.StringAttribute{
						MarkdownDescription: "A `bundle.json`, instead of `remote` or `repository`: its content is followed by `content_hash`.",
						Optional:            true,
					},
					"parameters": schema.MapAttribute{
						MarkdownDescription: "This installation's parameters, over the stack's.",
						ElementType:         types.StringType,
						Optional:            true,
					},
					"credentials": credentialsSchema("This installation's credentials, over the stack's."),
				}},
			},
			"purge_on_destroy": schema.BoolAttribute{
				MarkdownDescription: "Destroying the stack, or an installation it no longer lists, also removes their volumes, **data included**. Default: `false`, the volumes are kept for the next install.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true, Delete: true,
				CreateDescription: "How long the first apply may take, every installation included. Default: as long as nemo's actions take.",
				ReadDescription:   "How long reading the stack may take. Default: as long as it takes.",
				UpdateDescription: "How long an apply may take, every installation included. Default: as long as nemo's actions take.",
				DeleteDescription: "How long destroying the stack may take. Default: as long as nemo's actions take.",
			}),
			"content_hash": schema.StringAttribute{
				MarkdownDescription: "The SHA-256 of the installations' bundle files, in their order, computed at plan time: " +
					"a bundle rebuilt at the same path changes it, and the next apply deploys it.",
				Computed: true,
			},
			"manifest": schema.StringAttribute{
				MarkdownDescription: "The stack's manifest, in YAML, for `nemo stack import`: nemo then reads the stack, " +
					"its status and its logs, from where it reaches the target. Its paths are absolute, from where Terraform runs, " +
					"and its values, `value` or `value_wo`, are redacted, as nemo's records keep them: an action of nemo that needs one asks for it.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func targetSchema() schema.Attribute {
	options := schema.MapAttribute{
		MarkdownDescription: "The driver's options.",
		ElementType:         types.StringType,
		Optional:            true,
	}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Where the stack is deployed: one Docker, and the orchestrator that deploys there.",
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"orchestrator": schema.StringAttribute{
				MarkdownDescription: "`swarm` or `compose`. Changing it replaces the stack.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf(target.Swarm, target.Compose)},
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "The address of the target's Docker: `tcp://`, `ssh://user@host` or `unix://`. " +
					"With neither `host` nor `context`, the provider's, with its `tls`; else the environment's: `DOCKER_HOST`, the current docker context, or the local socket.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("context"))},
			},
			"context": schema.StringAttribute{
				MarkdownDescription: "A docker context to reach the target with, instead of `host`.",
				Optional:            true,
			},
			"tls": schema.SingleNestedAttribute{
				MarkdownDescription: "The TLS of a `tcp://` host, in PEM, instead of `DOCKER_CERT_PATH` and `DOCKER_TLS_VERIFY`. " +
					"Each part is read from a source, as a credential is, and the manifest keeps the source.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"ca":   sourceSchema("The CA of the daemon's certificate. Default: the system's.", false),
					"cert": sourceSchema("The client's certificate, with its key.", false),
					"key":  sourceSchema("The client's key, with its certificate.", false),
				},
			},
			"images": schema.StringAttribute{
				MarkdownDescription: "How the images reach the target: `registry`, which each node pulls them from, or `load`, into its one daemon. Default: `registry`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(target.ImagesRegistry),
				Validators:          []validator.String{stringvalidator.OneOf(target.ImagesRegistry, target.ImagesLoad)},
			},
			"volumes": schema.MapNestedAttribute{
				MarkdownDescription: "The target's volume types, by name: what a volume of a bundle with that `x-nemo-volume` type is made of. " +
					"The driver, its options and the provisioned directory are templates of `{{.Stack}}`, `{{.Name}}` and `{{.Options}}`. " +
					"The type `records` makes the volume of the stack's records, with `nemo-records` as its `{{.Name}}`: on a swarm of several managers, " +
					"a storage they all mount, where each finds the same records. Without it, the records are in a local volume of the Docker the provider reaches.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"driver":  schema.StringAttribute{MarkdownDescription: "The volume driver. Default: `local`.", Optional: true},
					"options": options,
					"provision": schema.SingleNestedAttribute{
						MarkdownDescription: "A directory to create for each volume before the volume, in the storage `volume` mounts.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"directory": schema.StringAttribute{MarkdownDescription: "The directory, in the storage.", Required: true},
							"volume": schema.SingleNestedAttribute{
								MarkdownDescription: "The storage the directory is created in.",
								Required:            true,
								Attributes: map[string]schema.Attribute{
									"driver":  schema.StringAttribute{MarkdownDescription: "The volume driver. Default: `local`.", Optional: true},
									"options": options,
								},
							},
						},
					},
				}},
			},
		},
	}
}

func installerSchema() schema.Attribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Where gill, the installer, runs: on the target, the default, or as a Job in a Kubernetes cluster that reaches the target over `tcp://`.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"driver": schema.StringAttribute{
				MarkdownDescription: "`docker` or `kubernetes`. Default: `docker`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf(target.DriverDocker, target.DriverKubernetes)},
			},
			"kubernetes": schema.SingleNestedAttribute{
				MarkdownDescription: "The cluster of the `kubernetes` driver.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"namespace":  schema.StringAttribute{MarkdownDescription: "The namespace of the Job.", Optional: true},
					"kubeconfig": schema.StringAttribute{MarkdownDescription: "The kubeconfig of the cluster. Default: `KUBECONFIG`, `~/.kube/config`, else the cluster Terraform runs in.", Optional: true},
				},
			},
		},
	}
}

// sourceAttributes are those of a source, as nemo reads a credential's:
// exactly one of a file, a variable, a command or a value, read where
// Terraform runs; a credential's value can be write-only.
func sourceAttributes(writeOnly bool) map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"path":    schema.StringAttribute{MarkdownDescription: "A file.", Optional: true},
		"env":     schema.StringAttribute{MarkdownDescription: "An environment variable.", Optional: true},
		"command": schema.StringAttribute{MarkdownDescription: "A command, whose output is the value.", Optional: true},
		"value":   schema.StringAttribute{MarkdownDescription: "The value itself, which the state keeps.", Optional: true, Sensitive: true},
	}
	if !writeOnly {
		return attributes
	}
	attributes["value_wo"] = schema.StringAttribute{
		MarkdownDescription: "The value itself, write-only: neither the plan nor the state keeps it, with Terraform 1.11 or later, from an ephemeral resource for instance. " +
			"Terraform can't compare it: change `value_wo_version` to give a new one.",
		Optional:  true,
		WriteOnly: true,
	}
	attributes["value_wo_version"] = schema.StringAttribute{
		MarkdownDescription: "Changed, the next apply gives `value_wo` again.",
		Optional:            true,
		Validators:          []validator.String{stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("value_wo"))},
	}
	return attributes
}

func sourceSchema(description string, required bool) schema.Attribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: description,
		Optional:            !required,
		Required:            required,
		Attributes:          sourceAttributes(false),
		Validators:          []validator.Object{oneSource{}},
	}
}

// credentialsSchema is a map of credential sources.
func credentialsSchema(description string) schema.Attribute {
	return schema.MapNestedAttribute{
		MarkdownDescription: description + " Each is read where Terraform runs, from one source.",
		Optional:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: sourceAttributes(true),
			Validators: []validator.Object{oneSource{}},
		},
	}
}

// ImportState takes a stack nemo deployed: its name, after the address of
// its Docker when the environment's is not the one, tcp://10.1.0.10:2376/shop.
// Reading the stack then fills the rest from its records on the target.
func (r *nemoStackResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	host, name, err := importID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	if host != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target").AtName("host"), host)...)
	}
}

func (r *nemoStackResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config nemoStackResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	copyWriteOnly(&plan, config)

	ctx, cancel := bounded(ctx, plan.Timeouts.Create, &resp.Diagnostics)
	defer cancel()
	manifest := expandStack(ctx, r.connection.over(plan), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	present, err := installedOn(ctx, manifest)
	if err != nil {
		resp.Diagnostics.AddError("nemo stack read failed", err.Error())
		return
	}
	if len(present) > 0 {
		resp.Diagnostics.AddError("nemo stack already exists",
			fmt.Sprintf("The target has installations of the stack %s already: import it, with terraform import and the ID %s.", manifest.Name, importIDOf(manifest)))
		return
	}

	plan.ID = types.StringValue(manifest.Name)
	plan.Manifest = encodeManifest(manifest, &resp.Diagnostics)
	if err := up(ctx, manifest, plan.PurgeOnDestroy.ValueBool()); err != nil {
		resp.Diagnostics.AddError("nemo stack create failed", err.Error())
	}
	// Saved even when an action failed: the stack is tainted, and the next apply replaces it.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nemoStackResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nemoStackResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := bounded(ctx, state.Timeouts.Read, &resp.Diagnostics)
	defer cancel()
	manifest := expandStack(ctx, r.connection.over(state), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	present, err := installedOn(ctx, manifest)
	if err != nil {
		resp.Diagnostics.AddError("nemo stack read failed", err.Error())
		return
	}
	if len(present) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	refreshInstallations(ctx, &state, present, &resp.Diagnostics)
	// An imported stack: its bundles' files as they are now.
	if state.ContentHash.IsNull() {
		hash, err := contentHash(installationFiles(state.Installations))
		if err != nil {
			resp.Diagnostics.AddError("nemo stack read failed", err.Error())
			return
		}
		state.ContentHash = hash
	}
	state.Manifest = encodeManifest(expandStack(ctx, r.connection.over(state), &resp.Diagnostics), &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nemoStackResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config nemoStackResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	copyWriteOnly(&plan, config)

	ctx, cancel := bounded(ctx, plan.Timeouts.Update, &resp.Diagnostics)
	defer cancel()
	manifest := expandStack(ctx, r.connection.over(plan), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(manifest.Name)
	plan.Manifest = encodeManifest(manifest, &resp.Diagnostics)
	if err := up(ctx, manifest, plan.PurgeOnDestroy.ValueBool()); err != nil {
		resp.Diagnostics.AddError("nemo stack update failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ModifyPlan computes content_hash from the bundle files the plan gives, so
// that a bundle rebuilt at the same path changes the plan, and loads the
// bundles the plan installs anew, so that one that is not there fails the
// plan rather than the apply.
func (r *nemoStackResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var installations types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("installations"), &installations)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hash := types.StringUnknown()
	if files, known := plannedFiles(installations); known {
		var err error
		if hash, err = contentHash(files); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("installations"), "nemo stack bundle unreadable", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_hash"), hash)...)

	// A plan with values an apply computes is checked by that apply.
	var planned, current nemoStackResourceModel
	if req.Plan.Get(ctx, &planned).HasError() || (!req.State.Raw.IsNull() && req.State.Get(ctx, &current).HasError()) {
		return
	}
	var ignored diag.Diagnostics
	checkBundles(ctx, planned, expandStack(ctx, r.connection.over(planned), &ignored), expandStack(ctx, r.connection.over(current), &ignored), &resp.Diagnostics)
}

func (r *nemoStackResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nemoStackResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := bounded(ctx, state.Timeouts.Delete, &resp.Diagnostics)
	defer cancel()
	manifest := expandStack(ctx, r.connection.over(state), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := down(ctx, manifest, state.PurgeOnDestroy.ValueBool()); err != nil {
		resp.Diagnostics.AddError("nemo stack delete failed", err.Error())
	}
}
