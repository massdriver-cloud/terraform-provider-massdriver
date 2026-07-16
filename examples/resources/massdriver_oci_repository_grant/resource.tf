# A sharing grant on an OCI repository. Each grant shares the repository
# with recipient projects matching `recipient_conditions`.
# Grants are immutable — changing any field replaces the grant.

# Let every project in the org pull from this repository.
resource "massdriver_oci_repository_grant" "pull_org_wide" {
  repository_id        = massdriver_oci_repository.aurora.id
  recipient_conditions = "*"
}

# Let only projects tagged with the eng team pull from this repository.
resource "massdriver_oci_repository_grant" "pull_for_eng" {
  repository_id        = massdriver_oci_repository.aurora.id
  recipient_conditions = jsonencode({ team = ["eng"] })
}
