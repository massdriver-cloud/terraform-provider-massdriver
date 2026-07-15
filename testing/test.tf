# Live smoke test — runs the locally-built provider against a real
# Massdriver org. Everything created here is prefixed "provider-live-test"
# and safe to destroy.
#
# Setup:
#   1. Build the provider binary (from the repo root):
#        make build
#   2. Export credentials for a sandbox org:
#        export MASSDRIVER_API_KEY=...
#        export MASSDRIVER_ORGANIZATION_ID=...
#        export MASSDRIVER_URL=...        # only if not targeting prod
#   3. Run terraform from this directory with the dev override
#      (skip `terraform init` — dev_overrides makes it unnecessary):
#        TF_CLI_CONFIG_FILE=dev.tfrc terraform plan
#        TF_CLI_CONFIG_FILE=dev.tfrc terraform apply
#        TF_CLI_CONFIG_FILE=dev.tfrc terraform destroy

terraform {
  required_providers {
    massdriver = {
      source = "massdriver-cloud/massdriver"
    }
  }
}

provider "massdriver" {}

# --- OCI repository + sharing grants ---------------------------------------

resource "massdriver_oci_repository" "test" {
  name          = "provider-live-test-repo"
  artifact_type = "BUNDLE"
  attributes = {
    team = "provider-live-test"
  }
}

# Wildcard grant: every project in the org may pull.
resource "massdriver_oci_repository_grant" "pull_org_wide" {
  repository_id        = massdriver_oci_repository.test.id
  recipient_conditions = "*"
}

# Conditional grant: only projects tagged with the test team may pull.
resource "massdriver_oci_repository_grant" "pull_for_team" {
  repository_id        = massdriver_oci_repository.test.id
  recipient_conditions = jsonencode({ team = ["provider-live-test"] })
}

# --- Imported resource + sharing grants ------------------------------------

resource "massdriver_imported_resource" "test" {
  name          = "provider-live-test-role"
  resource_type = "aws-iam-role"
  resource      = jsonencode({ arn = "arn:aws:iam::111111111111:role/provider-live-test" })
}

# Wildcard grant: every environment in the org may use the resource.
resource "massdriver_resource_grant" "export_org_wide" {
  resource_id          = massdriver_imported_resource.test.id
  recipient_conditions = "*"
}

# Conditional grant: only environments tagged with the test team.
resource "massdriver_resource_grant" "export_for_team" {
  resource_id          = massdriver_imported_resource.test.id
  recipient_conditions = jsonencode({ team = ["provider-live-test"] })
}

output "repository_reference" {
  value = massdriver_oci_repository.test.reference
}
