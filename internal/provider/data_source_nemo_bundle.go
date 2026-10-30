package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &nemoBundleDataSource{}

type nemoBundleDataSource struct{}

type nemoBundleDataSourceModel struct {
	Repository        types.String `tfsdk:"repository"`
	VersionConstraint types.String `tfsdk:"version_constraint"`
	Version           types.String `tfsdk:"version"`
	Versions          types.List   `tfsdk:"versions"`
}

func NewNemoBundleDataSource() datasource.DataSource {
	return &nemoBundleDataSource{}
}

func (d *nemoBundleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bundle"
}

func (d *nemoBundleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The versions of a bundle in a registry: its tags that are whole versions, `1.4.0` or `v1.4.0` but not `1.4` nor `latest`, and the latest that matches a constraint, " +
			"for an installation's `version`. The registry is reached as `docker` reaches it, with the credentials of `docker login`.",
		Attributes: map[string]schema.Attribute{
			"repository": schema.StringAttribute{
				MarkdownDescription: "The bundle's repository, as an installation's `repository`: `ghcr.io/sntns/whoami`.",
				Required:            true,
			},
			"version_constraint": schema.StringAttribute{
				MarkdownDescription: "The versions to choose from, as Terraform reads a version constraint: `~> 1.4` or `>= 1.2, < 2`. " +
					"Default: every version but the pre-releases, which only a constraint that names one matches.",
				Optional: true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The latest version that matches the constraint, as its tag spells it.",
				Computed:            true,
			},
			"versions": schema.ListAttribute{
				MarkdownDescription: "Every version that matches the constraint, from the oldest to the latest, as their tags spell them.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *nemoBundleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model nemoBundleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repository, err := name.NewRepository(model.Repository.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid nemo bundle repository", err.Error())
		return
	}
	tags, err := remote.List(repository, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		resp.Diagnostics.AddError("nemo bundle versions failed", err.Error())
		return
	}
	versions, err := matchingVersions(tags, model.VersionConstraint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("nemo bundle versions failed", fmt.Sprintf("%s: %s", repository, err))
		return
	}

	model.Version = types.StringValue(versions[len(versions)-1])
	list, diags := types.ListValueFrom(ctx, types.StringType, versions)
	resp.Diagnostics.Append(diags...)
	model.Versions = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// matchingVersions are the tags that are whole versions the constraint matches,
// as Terraform reads a version constraint, from the oldest to the latest,
// as spelt: a template of {{ .Version }} gives the tag back. A pre-release
// matches only a constraint that names one; a tag that moves, 1.4 or
// latest, none.
func matchingVersions(tags []string, constraint string) ([]string, error) {
	var matches version.Constraints
	if constraint != "" {
		var err error
		if matches, err = version.NewConstraint(constraint); err != nil {
			return nil, fmt.Errorf("version_constraint %q: %w", constraint, err)
		}
	}
	var versions []*version.Version
	for _, tag := range tags {
		given, err := version.NewVersion(tag)
		if err != nil || given.String() != strings.TrimPrefix(tag, "v") || (matches == nil && given.Prerelease() != "") || (matches != nil && !matches.Check(given)) {
			continue
		}
		versions = append(versions, given)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no tag is a version that matches %q", constraint)
	}
	slices.SortStableFunc(versions, func(a, b *version.Version) int { return a.Compare(b) })
	spelt := make([]string, len(versions))
	for i, given := range versions {
		spelt[i] = given.Original()
	}
	return spelt, nil
}
