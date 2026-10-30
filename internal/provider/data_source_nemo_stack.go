package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sntns/nemo/pkg/stack"
)

var (
	_ datasource.DataSource              = &nemoStackDataSource{}
	_ datasource.DataSourceWithConfigure = &nemoStackDataSource{}
)

type nemoStackDataSource struct {
	connection *nemoConnection
}

type nemoStackDataSourceModel struct {
	Name          types.String                     `tfsdk:"name"`
	Host          types.String                     `tfsdk:"host"`
	Context       types.String                     `tfsdk:"context"`
	Orchestrator  types.String                     `tfsdk:"orchestrator"`
	Installations []nemoStackDataInstallationModel `tfsdk:"installations"`
}

type nemoStackDataInstallationModel struct {
	Name       types.String `tfsdk:"name"`
	Version    types.String `tfsdk:"version"`
	Repository types.String `tfsdk:"repository"`
	File       types.String `tfsdk:"file"`
	Action     types.String `tfsdk:"action"`
	Result     types.String `tfsdk:"result"`
}

func NewNemoStackDataSource() datasource.DataSource {
	return &nemoStackDataSource{}
}

func (d *nemoStackDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (d *nemoStackDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if connection, ok := req.ProviderData.(*nemoConnection); ok {
		d.connection = connection
	}
}

func (d *nemoStackDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: description, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A stack its target has, deployed by nemo or by Terraform elsewhere: its installations, from their records on the target. " +
			"Reading it changes nothing.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{MarkdownDescription: "The stack's name.", Required: true},
			"host": schema.StringAttribute{
				MarkdownDescription: "The address of the target's Docker: `tcp://`, `ssh://user@host` or `unix://`. " +
					"With neither `host` nor `context`, the provider's, with its `tls`; else the environment's.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("context"))},
			},
			"context":      schema.StringAttribute{MarkdownDescription: "A docker context to reach the target with, instead of `host`.", Optional: true},
			"orchestrator": computed("`swarm` or `compose`, which deployed the stack."),
			"installations": schema.ListNestedAttribute{
				MarkdownDescription: "The stack's installations, those uninstalled left out.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name":       computed("The installation's name."),
					"version":    computed("The version of the bundle its last action ran."),
					"repository": computed("The bundle's repository, when it came from a registry."),
					"file":       computed("The bundle's `bundle.json`, when it came from one."),
					"action":     computed("Its last action, such as `install` or `upgrade`."),
					"result":     computed("How its last action went, as CNAB says it: `succeeded`, `failed`, `running`…"),
				}},
			},
		},
	}
}

func (d *nemoStackDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model nemoStackDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The stack as the resource reaches it: its own host or context, else the provider's.
	reached := d.connection.over(nemoStackResourceModel{Name: model.Name, Target: &nemoStackTargetModel{Host: model.Host, Context: model.Context}})
	manifest := stack.Stack{Name: model.Name.ValueString()}
	directory, err := os.Getwd()
	if err != nil {
		resp.Diagnostics.AddError("nemo stack paths failed", err.Error())
		return
	}
	manifest.Target = expandTarget(ctx, reached.Target, directory, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	present, err := installedOn(ctx, manifest)
	if err != nil {
		resp.Diagnostics.AddError("nemo stack read failed", err.Error())
		return
	}
	if len(present) == 0 {
		resp.Diagnostics.AddError("nemo stack not found", fmt.Sprintf("The target has no installation of the stack %s.", manifest.Name))
		return
	}

	model.Orchestrator = types.StringValue(present[0].Target.Orchestrator)
	model.Installations = nil
	for _, installation := range present {
		flat := flattenInstallation(installation)
		model.Installations = append(model.Installations, nemoStackDataInstallationModel{
			Name:       flat.Name,
			Version:    optionalString(deployedVersion(installation)),
			Repository: flat.Repository,
			File:       flat.File,
			Action:     optionalString(installation.Status.Action),
			Result:     optionalString(installation.Status.ResultStatus),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
