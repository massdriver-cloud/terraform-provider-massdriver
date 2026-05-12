# Replaces the deprecated `massdriver_package_alarm` resource. Migration:
#
#   resource "massdriver_package_alarm" "x" {    resource "massdriver_instance_alarm" "x" {
#     cloud_resource_id = "..."                    cloud_resource_id = "..."
#     display_name      = "..."                    display_name      = "..."
#     period_minutes    = 5                        period            = 300     # SECONDS now, not minutes
#     threshold         = 80                       threshold         = 80
#     comparison_operator = "GreaterThanThreshold" comparison_operator = "GreaterThanThreshold"
#     metric {                                     metric {
#       name = "..."                                 name = "..."
#       namespace = "..."                            namespace = "..."
#       statistic = "Average"                        statistic = "Average"
#       dimensions = { ... }                         dimensions = { ... }
#     }                                            }
#   }                                            }

# Bundle deployments: instance_id defaults from MASSDRIVER_INSTANCE_ID
# so it can be omitted entirely:
resource "massdriver_instance_alarm" "high_cpu_in_deployment" {
  display_name      = "High CPU Alarm"
  cloud_resource_id = aws_cloudwatch_metric_alarm.alarm.arn

  threshold           = 80
  period              = 300
  comparison_operator = "GreaterThanThreshold"

  metric {
    name      = "CPUUtilization"
    namespace = "AWS/RDS"
    statistic = "Average"
    dimensions = {
      DBInstanceIdentifier = aws_db_instance.main.id
    }
  }
}

# Outside a deployment (CI, local apply, etc.): instance_id is required.
# Set MASSDRIVER_INSTANCE_ID in the environment, or pass it explicitly:
resource "massdriver_instance_alarm" "high_cpu_explicit_instance" {
  instance_id       = "my-team-ecommerce-prod-db"
  display_name      = "High CPU Alarm"
  cloud_resource_id = aws_cloudwatch_metric_alarm.alarm.arn
}

# Alertmanager / other sources without structured metric data:
resource "massdriver_instance_alarm" "alertmanager_page" {
  display_name      = "Pager"
  cloud_resource_id = "alertmanager:my-alert"
}
