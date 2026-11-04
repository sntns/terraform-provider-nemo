package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cnabio/cnab-go/valuesource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage"
	"github.com/sntns/nemo/pkg/target"
)

func installation(name, version string) storage.Installation {
	installed := storage.Installation{
		InstallationSpec: storage.InstallationSpec{Name: name, Stack: "shop", Bundle: storage.BundleReference{Repository: "registry.example.com/" + name, Version: version}},
		Target:           target.Target{Orchestrator: target.Swarm, Configuration: target.Configuration{Host: "tcp://10.1.0.10:2376"}},
	}
	installed.Status.BundleVersion = version
	return installed
}

func TestRefreshInstallations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var diags diag.Diagnostics
	model := shop()
	refreshInstallations(ctx, &model, []storage.Installation{installation("api", "1.1.0"), installation("db", "16.0.0")}, &diags)
	require.False(t, diags.HasError(), diags)
	assert.Equal(t, []nemoStackInstallationModel{
		{Name: types.StringValue("api"), Version: types.StringValue("1.1.0"), Repository: types.StringNull(), File: types.StringNull(), Parameters: types.MapNull(types.StringType)},
		{Name: types.StringValue("db"), Repository: types.StringValue("registry.example.com/db"), Version: types.StringValue("16.0.0"), File: types.StringNull(), Parameters: types.MapNull(types.StringType)},
	}, model.Installations, "the version deployed by hand, web uninstalled, db installed by hand: the plan undoes them")
	assert.Equal(t, "shop", model.ID.ValueString())

	file := installation("api", "1.1.0")
	file.Bundle = storage.BundleReference{File: "/bundles/api/bundle.json"}
	model = nemoStackResourceModel{Name: types.StringValue("shop"), Installations: []nemoStackInstallationModel{
		{Name: types.StringValue("api"), Version: types.StringNull(), File: types.StringValue("/bundles/api/bundle.json")},
	}}
	refreshInstallations(ctx, &model, []storage.Installation{file}, &diags)
	assert.True(t, model.Installations[0].Version.IsNull(), "a file gives its own version: the plan has none to compare")
}

func TestRefreshInstallations_imported(t *testing.T) {
	t.Parallel()
	var diags diag.Diagnostics
	model := nemoStackResourceModel{Name: types.StringValue("shop"), Target: &nemoStackTargetModel{Host: types.StringValue("tcp://10.1.0.10:2376")}}
	refreshInstallations(context.Background(), &model, []storage.Installation{installation("api", "1.1.0")}, &diags)
	require.False(t, diags.HasError(), diags)
	assert.Equal(t, &nemoStackTargetModel{
		Orchestrator: types.StringValue(target.Swarm), Host: types.StringValue("tcp://10.1.0.10:2376"), Context: types.StringNull(),
		Images: types.StringValue(target.ImagesRegistry),
	}, model.Target)
	assert.Equal(t, []nemoStackInstallationModel{
		{Name: types.StringValue("api"), Repository: types.StringValue("registry.example.com/api"), Version: types.StringValue("1.1.0"), File: types.StringNull(), Parameters: types.MapNull(types.StringType)},
	}, model.Installations)
	assert.False(t, model.PurgeOnDestroy.ValueBool())
}

func TestImportID(t *testing.T) {
	t.Parallel()
	for id, want := range map[string][2]string{
		"shop":                           {"", "shop"},
		"tcp://10.1.0.10:2376/shop":      {"tcp://10.1.0.10:2376", "shop"},
		"ssh://deploy@prod-1/sntns-eu2":  {"ssh://deploy@prod-1", "sntns-eu2"},
		"unix:///var/run/docker.sock/it": {"unix:///var/run/docker.sock", "it"},
	} {
		host, name, err := importID(id)
		if assert.NoError(t, err, id) {
			assert.Equal(t, want, [2]string{host, name}, id)
		}
	}
	_, _, err := importID("tcp://10.1.0.10:2376/")
	assert.Error(t, err, "no name")
	_, _, err = importID("shop stack")
	assert.Error(t, err)
}

func TestContentHash(t *testing.T) {
	directory := t.TempDir()
	api, web, token := filepath.Join(directory, "api.json"), filepath.Join(directory, "web.json"), filepath.Join(directory, "token")
	require.NoError(t, os.WriteFile(api, []byte(`{"version":"1.0.0"}`), 0o600))
	require.NoError(t, os.WriteFile(web, []byte(`{"version":"2.0.0"}`), 0o600))
	require.NoError(t, os.WriteFile(token, []byte("t0ken"), 0o600))
	t.Setenv("TF_NEMO_TEST_KEY", "k3y")
	bundles := func(files ...string) stack.Stack {
		var manifest stack.Stack
		for _, file := range files {
			manifest.Installations = append(manifest.Installations, storage.InstallationSpec{Name: filepath.Base(file), Bundle: storage.BundleReference{File: file}})
		}
		return manifest
	}
	source := func(key, value string) []valuesource.Strategy {
		return []valuesource.Strategy{{Name: "api-key", Source: valuesource.Source{Key: key, Value: value}}}
	}
	hash := func(manifest stack.Stack) types.String {
		t.Helper()
		hash, err := contentHash(manifest)
		require.NoError(t, err)
		return hash
	}

	first := hash(bundles(api, web))
	assert.Equal(t, first, hash(bundles(api, web)))
	assert.NotEqual(t, first, hash(bundles(web, api)), "in the installations' order")
	require.NoError(t, os.WriteFile(api, []byte(`{"version":"1.0.1"}`), 0o600))
	assert.NotEqual(t, first, hash(bundles(api, web)), "a bundle rebuilt at the same path")
	_, err := contentHash(bundles(filepath.Join(directory, "missing.json")))
	assert.Error(t, err)

	given := bundles(api)
	given.Credentials = source("env", "TF_NEMO_TEST_KEY")
	given.Installations[0].Credentials = source("path", token)
	before := hash(given)
	assert.NotEqual(t, hash(bundles(api)), before)
	t.Setenv("TF_NEMO_TEST_KEY", "n3w")
	rotated := hash(given)
	assert.NotEqual(t, before, rotated, "a key rotated behind the same variable")
	require.NoError(t, os.WriteFile(token, []byte("n3w"), 0o600))
	assert.NotEqual(t, rotated, hash(given), "a file rewritten at the same path")

	unset := stack.Stack{Credentials: source("env", "TF_NEMO_TEST_UNSET")}
	missing := hash(unset)
	assert.False(t, missing.IsNull(), "a source that can't be read is left to the apply")
	t.Setenv("TF_NEMO_TEST_UNSET", "")
	assert.NotEqual(t, missing, hash(unset), "set, even empty")

	assert.True(t, hash(stack.Stack{Credentials: source("value", "k3y")}).IsNull(), "a value the state compares itself")
	assert.True(t, hash(stack.Stack{}).IsNull(), "a stack of registries only")
}

// The manifest output shows no value, as nemo's records keep none.
func TestEncodeManifest_redactsTheValues(t *testing.T) {
	t.Parallel()
	var diags diag.Diagnostics
	manifest := expandStack(context.Background(), shop(), &diags)
	manifest.Installations[0].Credentials = []valuesource.Strategy{{Name: "api-key", Source: valuesource.Source{Key: "value", Value: "k3y"}}}
	encoded := encodeManifest(manifest, &diags)
	require.False(t, diags.HasError(), diags)
	assert.NotContains(t, encoded.ValueString(), "k3y")
	assert.Contains(t, encoded.ValueString(), "value: '*******'", "the source kept, its value redacted")
	assert.Contains(t, encoded.ValueString(), "env: CERT", "the other sources as they are")
	assert.Equal(t, "key", manifest.Target.TLS.Key.Value, "the manifest nemo deploys keeps its values")
}
