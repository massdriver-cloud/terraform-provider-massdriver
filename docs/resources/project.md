# massdriver_project

A Massdriver project resource for organizing infrastructure and applications. Projects are the top-level organizational unit in Massdriver and contain environments, manifests, and packages.

## Example Usage

```hcl
resource "massdriver_project" "example" {
  name        = "My Application"
  slug        = "my-app"
  description = "Production application infrastructure"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) The name of the project. This is the human-readable display name.
* `slug` - (Required) The slug of the project. This is a unique identifier used in URLs and resource naming. Cannot be changed after creation.
* `description` - (Optional) A description of the project. Defaults to an empty string.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The unique identifier of the project.
* `last_updated` - A timestamp of when the resource was last updated.

## Import

Projects can be imported using the project ID or slug:

```shell
terraform import massdriver_project.example my-app
```

or using the project ID:

```shell
terraform import massdriver_project.example proj-123abc
```

