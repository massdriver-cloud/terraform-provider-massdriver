terraform {
  required_providers {
    massdriver = {
      version = "~> 1.0.0"
      source  = "massdriver-cloud/massdriver"
    }
  }
}

variable md_name_prefix {
  type        = string
  default     = "project-target-network-1234"
}


locals {
  aws_vpc = {
    main = {
      "arn" = "some fake arn"
      "id" = "some fake id"
    }
  }

  aws_iam_role = {
    "foo" = {
      "arn" = "some fake arn"
    }
  }
}

provider "massdriver" {}

resource "massdriver_artifact" "vpc" {

  # The field in the bundle's output artifacts schema.
  field                = "vpc"
  # The unique ID from the cloud provider
  provider_resource_id = local.aws_vpc.main.arn
  # The artifact definition type
  type                 = "aws-ec2-vpc"
  # A friendly name, overridable by user
  name                 = "VPC ${var.md_name_prefix} (${local.aws_vpc.main.id})"

  artifact = jsonencode(
    {      
      data = {
        infrastructure = {
          arn = local.aws_iam_role.foo.arn
        }
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

resource "massdriver_package_alarm" "high_cpu" {
  cloud_resource_id = "awshighcpualarm"
  display_name = "CPU Alarm 2"
}

output "artifact" {
  value = massdriver_artifact.vpc.artifact
  sensitive = true
}
