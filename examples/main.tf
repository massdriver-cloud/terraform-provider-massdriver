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
      field_name = "vpc"
      provider_resource_id = aws_vpc.main.arn
      type                 = "aws-ec2-vpc"
      name                 = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"
      data = { 
        arn = aws_iam_role.foo.arn
      }
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
