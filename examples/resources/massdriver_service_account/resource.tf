# A non-human identity for CI/CD and other API clients. The platform issues
# one token at creation, exposed as the sensitive `token` attribute.
resource "massdriver_service_account" "ci" {
  name                     = "ci-deployer"
  description              = "GitHub Actions deployer"
  token_expiration_minutes = 525600 # 1 year
}

# The token can't be re-fetched or re-issued in place, so rotate it by
# replacing the service account — here, automatically every 90 days.
resource "time_rotating" "ci_token" {
  rotation_days = 90
}

resource "massdriver_service_account" "rotated" {
  name                     = "ci-deployer-rotated"
  token_expiration_minutes = 60 * 24 * 100 # outlives the rotation window

  lifecycle {
    replace_triggered_by = [time_rotating.ci_token]
  }
}

output "ci_token" {
  value     = massdriver_service_account.ci.token
  sensitive = true
}
