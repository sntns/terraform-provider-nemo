package provider

import (
	"context"
	"testing"

	"github.com/cnabio/cnab-go/valuesource"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage"
	"github.com/sntns/nemo/pkg/target"
)

func source(key, value string) *nemoStackSourceModel {
	model := &nemoStackSourceModel{Path: types.StringNull(), Env: types.StringNull(), Command: types.StringNull(), Value: types.StringNull()}
	switch key {
	case "path":
		model.Path = types.StringValue(value)
	case "env":
		model.Env = types.StringValue(value)
	default:
		model.Value = types.StringValue(value)
	}
	return model
}

func stringMap(values map[string]string) types.Map {
	return flattenStrings(context.Background(), values, &diag.Diagnostics{})
}

// shop is a stack as Terraform plans it, with what the configuration
// leaves out null.
func shop() nemoStackResourceModel {
	return nemoStackResourceModel{
		ID: types.StringUnknown(), Name: types.StringValue("shop"), Manifest: types.StringUnknown(), PurgeOnDestroy: types.BoolValue(false),
		Target: &nemoStackTargetModel{
			Orchestrator: types.StringValue(target.Swarm), Host: types.StringValue("tcp://10.1.0.10:2376"), Context: types.StringNull(),
			Images: types.StringValue(target.ImagesRegistry),
			TLS:    &nemoStackTLSModel{CA: source("path", "ca.pem"), Cert: source("env", "CERT"), Key: source("value", "key")},
		},
		Remote:      types.StringValue("docker://registry.example.com/{{ .Component }}:{{ .Version }}"),
		Parameters:  types.MapNull(types.StringType),
		Credentials: map[string]nemoStackSourceModel{"token": *source("env", "TOKEN"), "registry": *source("path", "registry.json")},
		Installations: []nemoStackInstallationModel{
			{Name: types.StringValue("api"), Version: types.StringValue("1.2.0"), Repository: types.StringNull(), File: types.StringNull(), Parameters: types.MapNull(types.StringType)},
			{
				Name: types.StringValue("web"), Repository: types.StringValue("registry.example.com/web"), Version: types.StringValue("2.0.0"), File: types.StringNull(),
				Parameters: stringMap(map[string]string{"port": "8080"}),
			},
		},
	}
}

func TestExpandStack(t *testing.T) {
	t.Parallel()
	var diags diag.Diagnostics
	manifest := expandStack(context.Background(), shop(), &diags)
	require.False(t, diags.HasError(), diags)
	require.NoError(t, manifest.Validate())
	assert.Equal(t, stack.RecordsTarget, manifest.Records, "the records are on the target, for every machine that reaches it")
	assert.Equal(t, &target.TLS{
		CA:   &valuesource.Source{Key: "path", Value: "ca.pem"},
		Cert: &valuesource.Source{Key: "env", Value: "CERT"},
		Key:  &valuesource.Source{Key: "value", Value: "key"},
	}, manifest.Target.TLS)
	assert.Equal(t, []valuesource.Strategy{
		{Name: "registry", Source: valuesource.Source{Key: "path", Value: "registry.json"}},
		{Name: "token", Source: valuesource.Source{Key: "env", Value: "TOKEN"}},
	}, manifest.Credentials, "by name, the same manifest each time")
	assert.Equal(t, storage.InstallationSpec{
		Name: "web", Stack: "shop", Parameters: map[string]string{"port": "8080"},
		Bundle: storage.BundleReference{Repository: "registry.example.com/web", Version: "2.0.0"},
	}, manifest.Installations[1])
}

// The model holds every attribute of the schema, with the types the
// framework reads them into.
func TestNemoStackResourceModel_state(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var resp resource.SchemaResponse
	NewNemoStackResource().Schema(ctx, resource.SchemaRequest{}, &resp)
	state := tfsdk.State{Schema: resp.Schema}
	given := shop()
	given.Timeouts = timeouts.Value{Object: types.ObjectNull(resp.Schema.Attributes["timeouts"].GetType().(timeouts.Type).AttrTypes)}
	diags := state.Set(ctx, &given)
	require.False(t, diags.HasError(), diags)
	var read nemoStackResourceModel
	diags = state.Get(ctx, &read)
	require.False(t, diags.HasError(), diags)
	assert.Equal(t, expandStack(ctx, given, &diags), expandStack(ctx, read, &diags))
}

func TestFlattenTarget(t *testing.T) {
	t.Parallel()
	var diags diag.Diagnostics
	given := target.Target{Orchestrator: target.Compose, Configuration: target.Configuration{
		Host: "ssh://deploy@prod-1", TLS: &target.TLS{CA: &valuesource.Source{Key: "path", Value: "/certs/ca.pem"}},
		Volumes: map[string]target.VolumeType{"data": {VolumeDriver: target.VolumeDriver{Driver: "local", Options: map[string]string{"type": "nfs"}}}},
	}}
	flattened := flattenTarget(context.Background(), given, &diags)
	require.False(t, diags.HasError(), diags)
	assert.Equal(t, target.ImagesRegistry, flattened.Images.ValueString(), "nemo's default, as the schema's")
	expected := given
	expected.Images = target.ImagesRegistry
	assert.Equal(t, expected, expandTarget(context.Background(), flattened, &diags))
}
