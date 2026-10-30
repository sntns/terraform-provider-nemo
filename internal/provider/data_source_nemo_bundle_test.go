package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchingVersions(t *testing.T) {
	t.Parallel()
	tags := []string{"latest", "1.10.0", "v1.2.0", "1.9.3", "2.0.0-rc.1", "sha-1a2b3c", "1.4.0", "1.10", "01.2.3"}

	versions, err := matchingVersions(tags, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1.2.0", "1.4.0", "1.9.3", "1.10.0"}, versions, "in order, as tags spell them, without pre-releases nor tags that move")

	versions, err = matchingVersions(tags, "~> 1.4")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.4.0", "1.9.3", "1.10.0"}, versions)

	versions, err = matchingVersions(tags, ">= 2.0.0-rc.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"2.0.0-rc.1"}, versions, "a constraint that names a pre-release matches one")

	_, err = matchingVersions(tags, ">= 3")
	assert.ErrorContains(t, err, "no tag is a version")
	_, err = matchingVersions(tags, "not a constraint")
	assert.ErrorContains(t, err, "version_constraint")
}

// The versions of a public repository, whose tags never change: gill's,
// nemo's installer.
func TestAccNemoBundleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `data "nemo_bundle" "gill" {
  repository         = "ghcr.io/sntns/gill"
  version_constraint = "< 0.2"
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.nemo_bundle.gill", "version", "0.1.1"),
				resource.TestCheckResourceAttr("data.nemo_bundle.gill", "versions.#", "2"),
				resource.TestCheckResourceAttr("data.nemo_bundle.gill", "versions.0", "0.1.0"),
			),
		}},
	})
}
