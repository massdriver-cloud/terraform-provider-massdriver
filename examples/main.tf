terraform {
  required_providers {
    massdriver = {
      version = "0.0.1"
      source  = "massdriver.cloud/massdriver"
    }
  }
}

provider "massdriver" {}

resource "massdriver_artifact" "vpc" {

  # The field in the bundle's output artifacts schema.
  field                = "vpc"
  # The unique ID from the cloud provider
  provider_resource_id = aws_vpc.main.arn
  # The artifact definition type
  type                 = "aws-ec2-vpc"
  # A friendly name, overridable by user
  name                 = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"

  artifact = jsonencode(
    {      
      data = {
        infrastructure = {
          arn = aws_iam_role.foo.arn
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
  display_name = "High CPU Alarm"
  // AWS
  cloud_resource_id = aws_cloudwatch_metric_alarm.alarm.arn
  // GCP
  cloud_resource_id = google_monitoring_alert_policy.alert_policy.name
  // Azure
  cloud_resource_id = azurerm_monitor_metric_alert.main.id
}

output "artifact" {
  value = resource.massdriver_artifact.artifact
}