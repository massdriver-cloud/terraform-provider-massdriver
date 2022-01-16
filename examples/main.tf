terraform {
  required_providers {
    massdriver = {
      version = "0.1"
      source  = "massdriver.cloud/massdriver"
    }
  }
}

provider "massdriver" {}

resource "massdriver_artifact" {
  artifact = jsonencode({"hello"="world"})
}

output "artifact" {
  value = resource.massdriver_artifact.artifact
}
