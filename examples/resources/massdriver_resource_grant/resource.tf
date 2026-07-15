# A sharing grant on a resource. Each grant shares the resource with
# recipient environments matching `recipient_conditions`.
# Grants are immutable — changing any field replaces the grant.

# Share a resource with every environment in the org.
resource "massdriver_resource_grant" "share_org_wide" {
  resource_id          = massdriver_imported_resource.vpc.id
  recipient_conditions = "*"
}

# Share a resource only with environments tagged with the eng team.
resource "massdriver_resource_grant" "share_with_eng" {
  resource_id          = massdriver_imported_resource.vpc.id
  recipient_conditions = jsonencode({ team = ["eng"] })
}
