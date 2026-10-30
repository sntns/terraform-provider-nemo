package provider

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/stretchr/testify/require"
)

// A stack the resource deployed, found by terraform query on the
// environment's Docker, with its identity.
func TestAccNemoStackList(t *testing.T) {
	name := "tf-acc-list"
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	identity := map[string]knownvalue.Check{"name": knownvalue.StringExact(name), "host": knownvalue.Null()}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{
			{Config: fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_basic"), name, bundle)},
			{
				Query: true,
				Config: `
provider "nemo" {}
list "nemo_stack" "here" {
  provider = nemo
}`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectIdentity("nemo_stack.here", identity),
					querycheck.ExpectResourceDisplayName("nemo_stack.here", queryfilter.ByResourceIdentity(identity), knownvalue.StringExact(name)),
				},
			},
		},
	})
}
