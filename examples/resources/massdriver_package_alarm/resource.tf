resource "massdriver_package_alarm" "high_cpu" {
  display_name = "High CPU Alarm"
  cloud_resource_id = aws_cloudwatch_metric_alarm.alarm.arn
}