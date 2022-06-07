# Terraform Provider Massdriver

This provider allows us to manage massdriver artifacts generated from bundles right alongside the other resources in the bundle. It gives us delineated lifecycle events (create, update, destroy).

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
