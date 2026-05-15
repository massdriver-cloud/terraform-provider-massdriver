# Registers a cloud metric alarm with a deployed Massdriver instance.
# State updates arrive via webhook from CloudWatch / Azure Monitor / GCP
# Cloud Monitoring / Prometheus Alertmanager — the `cloud_resource_id`
# correlates inbound webhooks back to this record.

# Inside a Massdriver bundle deployment, `instance_id` defaults from
# MASSDRIVER_INSTANCE_ID, so you can omit it entirely:
resource "massdriver_instance_alarm" "high_cpu_in_bundle" {
  display_name      = "High CPU"
  cloud_resource_id = aws_cloudwatch_metric_alarm.cpu.arn

  comparison_operator = "GreaterThanThreshold"
  threshold           = 80
  period              = 300 # seconds

  metric {
    name      = "CPUUtilization"
    namespace = "AWS/RDS"
    statistic = "Average"
    dimensions = {
      DBInstanceIdentifier = aws_db_instance.primary.id
    }
  }
}

# Outside a deployment (CI, local apply): `instance_id` is required.
resource "massdriver_instance_alarm" "explicit_instance" {
  instance_id       = "ecomm-prod-db"
  display_name      = "High CPU"
  cloud_resource_id = aws_cloudwatch_metric_alarm.cpu.arn
}

# Alertmanager and similar sources without structured metric data: leave
# the metric block off entirely.
resource "massdriver_instance_alarm" "alertmanager_page" {
  display_name      = "PagerDuty Alert"
  cloud_resource_id = "alertmanager:my-alert"
}
