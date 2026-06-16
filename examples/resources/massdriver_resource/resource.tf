# Declares a connectable resource produced by a Massdriver bundle. Use
# this only inside the IaC of a Massdriver bundle to satisfy a resource
# slot declared in the bundle's massdriver.yaml; outside a deployment the
# provider fast-fails on the missing MASSDRIVER_DEPLOYMENT_ID / _TOKEN
# env vars.
#
# For resources that are NOT produced by a Massdriver bundle, use
# massdriver_imported_resource instead.

resource "massdriver_resource" "vpc" {
  # The `field` name from the bundle's massdriver.yaml `resources.properties`.
  field = "vpc"
  name  = "VPC ${var.md_name_prefix} (${aws_vpc.main.id})"

  # JSON-encoded resource data, validated locally against
  # schema-artifacts.json before being sent.
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
