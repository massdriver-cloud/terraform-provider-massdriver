# The Massdriver provider has no HCL-configurable attributes. All settings
# are read from MASSDRIVER_* environment variables.
#
# An empty `provider "massdriver" {}` block is all that's required. In a
# Massdriver bundle deployment, no environment configuration is needed
# either — the platform injects MASSDRIVER_DEPLOYMENT_ID + MASSDRIVER_TOKEN
# into the deployment environment automatically, and the provider picks
# them up.

terraform {
  required_providers {
    massdriver = {
      source  = "massdriver-cloud/massdriver"
      version = "~> 2.0"
    }
  }
}

provider "massdriver" {}
