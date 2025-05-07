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
          region   = "us-west-2"
        }
      }
    }
  )
}