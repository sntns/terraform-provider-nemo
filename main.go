// The nemo provider deploys a stack of bundles from the manifest nemo
// reads, with nemo's own library.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/sntns/terraform-provider-nemo/internal/provider"
)

//go:generate go tool tfplugindocs generate --provider-name nemo

// version is the release's, stamped by GoReleaser.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider for a debugger, such as delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/sntns/nemo",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err)
	}
}
