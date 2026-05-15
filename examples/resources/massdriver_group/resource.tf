# A custom access-control group within the organization. Members gain
# permissions through ABAC policies attached via massdriver_group_policy.
#
# Built-in groups (organization_admin, organization_viewer) are managed
# by the platform and cannot be created or destroyed here.

resource "massdriver_group" "sres" {
  name        = "SREs"
  description = "On-call engineers for prod incidents."
}
