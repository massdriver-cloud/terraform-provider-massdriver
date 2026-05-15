# A bundle slot in a project's blueprint. Each component is sourced from
# an OCI repository (the published bundle). One component, deployed across
# multiple environments, produces one instance per environment.

resource "massdriver_component" "database" {
  project_id  = massdriver_project.ecommerce.id
  identifier  = "db"
  name        = "Primary Database"
  bundle_name = "aws-rds-cluster"

  # Component-scope attributes (e.g., `pci`, `soc2`) are configured per-org
  # in the Massdriver console; pass an empty map when none are required.
  attributes = {}
}
