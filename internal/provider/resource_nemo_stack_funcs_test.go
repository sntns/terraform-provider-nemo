package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
