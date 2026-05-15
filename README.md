# Terraform Provider for Massdriver

The official Terraform / OpenTofu provider for the [Massdriver](https://www.massdriver.cloud) platform. Manages projects, environments, components, groups, policies, OCI repositories, and the bundle-side resources (`massdriver_resource`, `massdriver_instance_alarm`, `massdriver_imported_resource`) — alongside the rest of the infrastructure declared in a bundle's IaC.

## Documentation

Per-resource docs live under [`docs/resources/`](./docs/resources/) and are mirrored on the [Terraform Registry](https://registry.terraform.io/providers/massdriver-cloud/massdriver/latest/docs) and the [OpenTofu Registry](https://search.opentofu.org/provider/massdriver-cloud/massdriver/latest).

See [CHANGELOG.md](./CHANGELOG.md) for release notes and version-to-version migration guides. v2.0.0 is a breaking release — if you're coming from v1.x, the CHANGELOG entry lists exactly what to rename.

## Usage

```hcl
terraform {
  required_providers {
    massdriver = {
      source  = "massdriver-cloud/massdriver"
      version = "~> 2.0"
    }
  }
}

provider "massdriver" {}

resource "massdriver_project" "example" {
  identifier = "example"
  name       = "Example"
  attributes = {
    team = "platform"
  }
}
```

The provider reads its credentials and target from the standard `MASSDRIVER_*` environment variables resolved by the [Go SDK](https://github.com/massdriver-cloud/massdriver-sdk-go) — `MASSDRIVER_API_KEY`, `MASSDRIVER_ORGANIZATION_ID`, `MASSDRIVER_URL` for platform-side resources; `MASSDRIVER_DEPLOYMENT_ID` + `MASSDRIVER_TOKEN` (injected automatically by the platform) for `massdriver_resource` running inside a bundle deployment.

## Development

A dev container is included for VSCode users — install the [remote-containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) extension and use **"Remote-Containers: Open Folder In Container"** to get the Go toolchain and tofu CLI ready to go.

### Build

```sh
make build      # build a single binary into the working dir
make install    # build + install into ~/.terraform.d/plugins/ for all OS_ARCHS
```

### Test

```sh
go test ./...   # unit tests (no live API needed)
```

For live API tests, point the provider at a sandbox org via a `~/.tofurc` `dev_overrides` block and apply the configuration under [`testing/`](./testing/) — see the comments in `testing/test.tf` for the setup.

### Regenerate docs

```sh
make docs
```

Runs [`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs) against the schema + the example `.tf` files under [`examples/resources/`](./examples/resources/) to regenerate the per-resource pages under `docs/`. Requires the provider to be installed locally (via `make install`).
