# NOTE: massdriver_package_alarm is deprecated. Use massdriver_instance_alarm instead.
# This resource will be removed in v2.0 of the provider.
# See the v1.3.0 CHANGELOG entry for a side-by-side migration example.

resource "massdriver_package_alarm" "high_cpu" {
  display_name      = "High CPU Alarm"
  cloud_resource_id = aws_cloudwatch_metric_alarm.alarm.arn
}
