// Package provider is the nemo provider: a stack resource that nemo's
// library deploys, as the nemo CLI deploys a manifest.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &nemoProvider{}

type nemoProvider struct {
	version string
}

// nemoConnection is the Docker the provider reaches, for every stack whose
// target gives neither host nor context.
type nemoConnection struct {
	Host    types.String       `tfsdk:"host"`
	Context types.String       `tfsdk:"context"`
	TLS     *nemoStackTLSModel `tfsdk:"tls"`
}

// New returns the provider of the version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &nemoProvider{version: version}
	}
}

func (p *nemoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName, resp.Version = "nemo", p.version
}

func (p *nemoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	source := func(description string) schema.Attribute {
		return schema.SingleNestedAttribute{
			MarkdownDescription: description,
			Optional:            true,
			Attributes: map[string]schema.Attribute{
				"path":    schema.StringAttribute{MarkdownDescription: "A file.", Optional: true},
				"env":     schema.StringAttribute{MarkdownDescription: "An environment variable.", Optional: true},
				"command": schema.StringAttribute{MarkdownDescription: "A command, whose output is the value.", Optional: true},
				"value":   schema.StringAttribute{MarkdownDescription: "The value itself.", Optional: true, Sensitive: true},
			},
			Validators: []validator.Object{oneSource{}},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Deploys stacks of bundles with [nemo](https://github.com/sntns/nemo): " +
			"each `nemo_stack` is a manifest nemo reads, deployed on the Docker of its target with Swarm or Compose. " +
			"The provider keeps the records of a stack's installations on its target, where `nemo status` and `nemo logs` find them " +
			"from any machine that imported the stack's `manifest`, and marks the stack as Terraform's: nemo writes it only with `--force`. " +
			"Its `host` or `context`, with its `tls`, is the Docker of every stack whose `target` gives neither `host` nor `context`.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "The address of the Docker of the stacks: `tcp://`, `ssh://user@host` or `unix://`. " +
					"With neither `host` nor `context`, the environment's: `DOCKER_HOST`, the current docker context, or the local socket.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("context"))},
			},
			"context": schema.StringAttribute{
				MarkdownDescription: "A docker context to reach the Docker of the stacks with, instead of `host`.",
				Optional:            true,
			},
			"tls": schema.SingleNestedAttribute{
				MarkdownDescription: "The TLS of a `tcp://` host, in PEM, for the stacks that give no `target.tls`. Each part is read from a source, as a credential is.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"ca":   source("The CA of the daemon's certificate. Default: the system's."),
					"cert": source("The client's certificate, with its key."),
					"key":  source("The client's key, with its certificate."),
				},
			},
		},
	}
}

// Configure gives the resources the provider's Docker. It must be known:
// a stack read on another Docker would be gone from the state.
func (p *nemoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var connection nemoConnection
	resp.Diagnostics.Append(req.Config.Get(ctx, &connection)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{"host": connection.Host, "context": connection.Context} {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown nemo provider "+name,
				"The provider reaches the Docker of the stacks at every plan: its "+name+" must be known then, not computed by an apply.")
		}
	}
	resp.ResourceData = &connection
	resp.DataSourceData = &connection
}

func (p *nemoProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewNemoStackResource}
}

func (p *nemoProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewNemoBundleDataSource, NewNemoStackDataSource}
}

// over is the stack reaching the provider's Docker when its target gives
// neither host nor context. The stack's own model is left as it is: the
// state keeps its configuration, not the provider's.
func (connection *nemoConnection) over(model nemoStackResourceModel) nemoStackResourceModel {
	if connection == nil || model.Target == nil || !model.Target.Host.IsNull() || !model.Target.Context.IsNull() {
		return model
	}
	given := *model.Target
	given.Host, given.Context = connection.Host, connection.Context
	if given.TLS == nil {
		given.TLS = connection.TLS
	}
	model.Target = &given
	return model
}
