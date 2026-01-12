# massdriver_environment

A Massdriver environment resource within a project. Environments represent deployment targets like production, staging, or development.

## Example Usage

```hcl
resource "massdriver_project" "ecommerce" {
  name        = "E-Commerce Platform"
  slug        = "ecommerce"
  description = "Main e-commerce application"
}

resource "massdriver_environment" "production" {
  project_id  = massdriver_project.ecommerce.id
  name        = "Production"
  slug        = "prod"
  description = "Production environment"
}

resource "massdriver_environment" "staging" {
  project_id  = massdriver_project.ecommerce.id
  name        = "Staging"
  slug        = "staging"
  description = "Staging environment for testing"
}
```

## Argument Reference

The following arguments are supported:

* `project_id` - (Required) The ID of the project this environment belongs to. Cannot be changed after creation.
* `name` - (Required) The name of the environment. This is the human-readable display name.
* `slug` - (Required) The slug of the environment. This forms part of the resource naming convention: `project-slug`. Cannot be changed after creation.
* `description` - (Optional) A description of the environment. Defaults to an empty string.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The unique identifier of the environment.
* `last_updated` - A timestamp of when the resource was last updated.

## Slug Convention

Massdriver uses a hierarchical slug pattern:
- Project: `project-slug`
- Environment: `project-env` (e.g., `ecommerce-prod`)
- Package: `project-env-package` (e.g., `ecommerce-prod-database`)

## Import

Environments can be imported using the environment ID:

```shell
terraform import massdriver_environment.production env-123abc
```
