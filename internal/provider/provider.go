// Package provider is the nemo provider: a stack resource that nemo's
// library deploys, as the nemo CLI deploys a manifest.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = (*nemoProvider)(nil)

type nemoProvider struct {
	version string
}

// New returns the provider of the version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &nemoProvider{version: version}
	}
}

func (p *nemoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, response *provider.MetadataResponse) {
	response.TypeName, response.Version = "nemo", p.version
}

func (p *nemoProvider) Schema(_ context.Context, _ provider.SchemaRequest, response *provider.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Deploys stacks of bundles with [nemo](https://github.com/sntns/nemo): " +
			"each `nemo_stack` is a manifest nemo reads, deployed on the Docker of its target with Swarm or Compose. " +
			"The provider keeps the records of a stack's installations on its target, where `nemo status` and `nemo logs` find them " +
			"from any machine that imported the stack's `manifest`, and marks the stack as Terraform's: nemo writes it only with `--force`.",
	}
}

func (p *nemoProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *nemoProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newStackResource}
}

func (p *nemoProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
