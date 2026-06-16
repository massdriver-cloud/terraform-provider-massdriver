# A repository in the Massdriver OCI catalog. Repositories must exist
# before any version can be published to them; pushing to a non-existent
# repository returns 404 from the registry.
#
# `artifact_type` is the kind of artifact stored. `BUNDLE` is the supported
# value today; resource-type and provisioner repository types are planned.

resource "massdriver_oci_repository" "aws_rds_cluster" {
  name          = "aws-rds-cluster"
  artifact_type = "BUNDLE"

  # Repository-scope attributes are configured per-organization; pass an
  # empty map when none are required.
  attributes = {}
}
