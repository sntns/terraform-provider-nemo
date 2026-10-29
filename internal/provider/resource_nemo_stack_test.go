package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sntns/nemo/pkg/runtime"
	"github.com/sntns/nemo/pkg/stack"
	"github.com/sntns/nemo/pkg/storage/volume"
	"github.com/sntns/nemo/pkg/target"
)

func TestNemoStackResource_schema(t *testing.T) {
	t.Parallel()
	var resp fwresource.SchemaResponse
	NewNemoStackResource().Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
	assert.True(t, resp.Schema.Attributes["manifest"].IsSensitive(), "it holds the credentials given by value")
}

// A stack on the environment's Docker, with Compose: created, updated,
// imported, refused a second time, then destroyed.
func TestAccNemoStack_basic(t *testing.T) {
	name := "tf-acc-basic"
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)
	config := func(test string) string {
		return fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", test), name, bundle)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: config("testAccNemoStack_basic"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nemo_stack.test", "id", name),
					resource.TestCheckResourceAttr("nemo_stack.test", "target.images", target.ImagesRegistry),
					resource.TestCheckResourceAttrSet("nemo_stack.test", "manifest"),
					checkNemoStackInstalled(name, "hello"),
				),
			},
			{
				Config: config("testAccNemoStack_update"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nemo_stack.test", "parameters.nemo-timeout", "6m"),
					checkNemoStackInstalled(name, "hello"),
				),
			},
			{
				ResourceName:      "nemo_stack.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The records keep the parameters nemo resolved, not those the configuration gives.
				ImportStateVerifyIgnore: []string{"parameters", "manifest"},
			},
			{
				Config:      config("testAccNemoStack_twin"),
				ExpectError: regexp.MustCompile("nemo stack already exists"),
			},
		},
	})
}

// checkNemoStackInstalled checks that the stack's records on the target
// have these installations, and only these.
func checkNemoStackInstalled(name string, installations ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		present, err := installedOn(context.Background(), stack.Stack{Name: name})
		if err != nil {
			return err
		}
		var names []string
		for _, installation := range present {
			names = append(names, installation.Name)
		}
		if !slices.Equal(names, installations) {
			return fmt.Errorf("stack %s: installed %v, expected %v", name, names, installations)
		}
		return nil
	}
}

// checkNemoStackDestroyed checks that nothing of the stack is installed any
// more, then removes its records from the target.
func checkNemoStackDestroyed(name string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		if err := checkNemoStackInstalled(name)(state); err != nil {
			return err
		}
		api, err := runtime.Connect(target.Target{})
		if err != nil {
			return err
		}
		defer api.Close()
		_, err = api.VolumeRemove(context.Background(), volume.Name(name), client.VolumeRemoveOptions{})
		return err
	}
}
