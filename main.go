package main

import (
	"log"

	"terraform-provider-massdriver/massdriver"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
)

func main() {
	var serveOpts []tf6server.ServeOpt
	err := tf6server.Serve(
		"registry.terraform.io/massdriver-cloud/massdriver",
		massdriver.ProviderServer,
		serveOpts...,
	)
	if err != nil {
		log.Fatal(err)
	}
}
