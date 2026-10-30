package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sntns/nemo/pkg/runtime"
	"github.com/sntns/nemo/pkg/target"
)

// testAccProtoV6ProviderFactories serve the provider to the Terraform CLI of
// each acceptance test.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nemo": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck checks that the environment's Docker answers: the
// acceptance tests deploy there.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	api, err := runtime.Connect(target.Target{})
	require.NoError(t, err)
	defer api.Close()
	_, err = api.Ping(context.Background(), client.PingOptions{})
	require.NoError(t, err, "the acceptance tests deploy on the environment's Docker")
}

func TestNemoProvider_schema(t *testing.T) {
	t.Parallel()
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	assert.False(t, resp.Schema.ValidateImplementation(context.Background()).HasError(), "a schema the framework serves")
}

func TestNemoConnection_over(t *testing.T) {
	t.Parallel()
	tls := &nemoStackTLSModel{CA: &nemoStackTLSSourceModel{Path: types.StringValue("ca.pem")}}
	connection := &nemoConnection{Host: types.StringValue("tcp://10.1.0.10:2376"), Context: types.StringNull(), TLS: tls}
	stack := func(host string) nemoStackResourceModel {
		target := &nemoStackTargetModel{Orchestrator: types.StringValue("swarm"), Host: types.StringNull(), Context: types.StringNull()}
		if host != "" {
			target.Host = types.StringValue(host)
		}
		return nemoStackResourceModel{Target: target}
	}

	given := stack("")
	over := connection.over(given)
	assert.Equal(t, "tcp://10.1.0.10:2376", over.Target.Host.ValueString(), "a stack without host reaches the provider's")
	assert.Same(t, tls, over.Target.TLS, "with the provider's TLS")
	assert.True(t, given.Target.Host.IsNull(), "the stack's model, which the state keeps, is left as it is")

	own := stack("ssh://deploy@prod-1")
	assert.Equal(t, own, connection.over(own), "a stack's own host comes over the provider's")
	assert.Equal(t, given, (*nemoConnection)(nil).over(given), "a provider not configured yet changes nothing")
}

// A stack whose target gives no host, deployed on the provider's: the
// environment's Docker, by its address.
func TestAccNemoStack_providerHost(t *testing.T) {
	name := "tf-acc-provider"
	api, err := runtime.Connect(target.Target{})
	require.NoError(t, err)
	host := api.DaemonHost()
	require.NoError(t, api.Close())
	bundle, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bundles", "hello", "bundle.json"))
	require.NoError(t, err)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNemoStackDestroyed(name),
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(loadTestConfiguration(t, "nemo_stack", "testAccNemoStack_provider"), name, bundle, host),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckNoResourceAttr("nemo_stack.test", "target.host"),
				resource.TestMatchResourceAttr("nemo_stack.test", "manifest", regexp.MustCompile(regexp.QuoteMeta("host: "+host))),
				checkNemoStackInstalled(name, "hello"),
			),
		}},
	})
}
