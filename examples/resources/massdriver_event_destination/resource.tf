variable "webhook_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "massdriver_event_destination" "production_deployments" {
  name    = "Production deployments"
  adapter = "HTTP"
  config = jsonencode({
    url     = "https://hooks.example.com/massdriver"
    headers = { "X-Team" = "platform" }
  })

  auth = jsonencode({
    headers = { Authorization = "Bearer ${var.webhook_token}" }
  })
  auth_version = 1

  event_types = ["deployment.completed", "deployment.failed"]
  attributes  = { "md-environment" = "prod" }
  condition   = ".data.metadata.deployment_action == \"PROVISION\""
}
