# Terraform Provider Massdriver

This is version 0.0.0.0.0.0.0.0.1. It is not ready to be released publicly.

This provider allows us to manage massdriver artifacts generated from bundles right alongside the other resources in the bundle. It gives us delineated lifecycle events (create, update, destroy). It also stops us from having to write artifact files to disk, then read from disk to upload via `xo`.

However, it also violates the trust boundary of terraform since there is no validation that the create/update/destroy was properly executed by Massdriver.

## Improvements

Eventually, this provider should change to use the low-level [terraform-plugin-go](https://github.com/hashicorp/terraform-plugin-go) and use dynamic type system to generate a rich type schema based on public hosted Massdriver jsonschema types. This will allow us to do jsonschema type validation through terraform built-in type system.

## Guide

Run the following command to build the provider

```shell
go build -o terraform-provider-massdriver
```

## Test sample configuration

First, build and install the provider.

```shell
make install
```

Then, run the following command to initialize the workspace and apply the sample configuration.

```shell
terraform init && terraform apply
```