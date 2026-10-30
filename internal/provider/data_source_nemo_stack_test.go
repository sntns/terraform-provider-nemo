package provider

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// A stack read by its name, after the resource deployed it.
func TestAccNemoStackDataSource(t *testing.T) {
	name := "tf-acc-data"
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_data"), name, bundle),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.nemo_stack.read", "orchestrator", "compose"),
				resource.TestCheckResourceAttr("data.nemo_stack.read", "installations.#", "1"),
				resource.TestCheckResourceAttr("data.nemo_stack.read", "installations.0.name", "hello"),
				resource.TestCheckResourceAttr("data.nemo_stack.read", "installations.0.version", "0.1.0"),
				resource.TestCheckResourceAttr("data.nemo_stack.read", "installations.0.file", bundle),
				resource.TestCheckResourceAttr("data.nemo_stack.read", "installations.0.result", "succeeded"),
			),
		}},
	})
}
