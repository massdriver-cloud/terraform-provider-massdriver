# Replaces the deprecated `massdriver_artifact` resource. Migration from
# artifact is mostly mechanical:
#
#   resource "massdriver_artifact" "vpc" {       resource "massdriver_resource" "vpc" {
#     field    = "vpc"                             field    = "vpc"
#     name     = "..."                             name     = "..."
#     artifact = jsonencode({...})                 resource = jsonencode({...})
#   }                                            }
#
# - rename the resource type from `massdriver_artifact` to `massdriver_resource`
# - rename the `artifact` argument to `resource`
# - remove `provider_resource_id` field

resource "massdriver_resource" "vpc" {
  # The field under the `resources` (formerly `artifacts`) block in massdriver.yaml
  field = "vpc"
  # A friendly name, overridable by user
  name = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"

  resource = jsonencode(
    {
      data = {
        infrastructure = {
          arn = aws_vpc.main.arn
        }
      }
      specs = {
        aws = {
          region = "us-west-2"
        }
      }
    }
  )
}
