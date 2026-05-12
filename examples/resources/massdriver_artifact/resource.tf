# NOTE: massdriver_artifact is deprecated. Use massdriver_resource instead.
# This resource will be removed in v2.0 of the provider.
# See the v1.3.0 CHANGELOG entry for a side-by-side migration example.

resource "massdriver_artifact" "vpc" {

  # The field under the `artifacts` block in massdriver.yaml
  field = "vpc"
  # A friendly name, overridable by user
  name = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"

  artifact = jsonencode(
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
