terraform {
  required_providers {
    massdriver = {
      version = "0.1"
      source  = "massdriver.cloud/massdriver"
    }
  }
}

provider "massdriver" {}

resource "massdriver_artifact" "vpc" {
  artifact = jsonencode(
    {
      # The field in the bundle's output artifacts schema.
      field = "vpc"
      # The unique ID from the cloud provider
      provider_resource_id = aws_vpc.main.arn
      # The artifact definition type
      type                 = "aws-ec2-vpc"
      # A friendly name, overridable by user
      name                 = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"
      # secret data
      data = { 
        arn = aws_iam_role.foo.arn
      }
      # search / filtering / matching specs
      specs = {
        aws = {
          region = "us-west-2"
          service = "iam"
          resource = "role"
        }
      }      
    }
  )
}

output "artifact" {
  value = resource.massdriver_artifact.artifact
}
