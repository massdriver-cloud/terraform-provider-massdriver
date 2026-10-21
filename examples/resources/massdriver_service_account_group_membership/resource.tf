# A new service account has no permissions until it joins a group.
resource "massdriver_group" "deployers" {
  name = "Deployers"
}

resource "massdriver_service_account" "ci" {
  name                     = "ci-deployer"
  token_expiration_minutes = 525600
}

resource "massdriver_service_account_group_membership" "ci_deployers" {
  group_id           = massdriver_group.deployers.id
  service_account_id = massdriver_service_account.ci.id
}
