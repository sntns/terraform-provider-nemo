package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
)

func TestNemoStackResource_schema(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewNemoStackResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	assert.True(t, resp.Schema.Attributes["manifest"].IsSensitive(), "it holds the credentials given by value")
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
