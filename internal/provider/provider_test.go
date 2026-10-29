package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/moby/moby/client"
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
