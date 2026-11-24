data "massdriver_event_types" "all" {}

locals {
  event_types = ["deployment.completed", "deployment.failed"]
}

resource "massdriver_event_destination" "audit" {
  name        = "Audit"
  adapter     = "HTTP"
  config      = jsonencode({ url = "https://hooks.example.com/massdriver" })
  event_types = local.event_types

  lifecycle {
    precondition {
      condition     = alltrue([for t in local.event_types : contains(data.massdriver_event_types.all.ids, t)])
      error_message = "Every event type must be in the event catalog."
    }
  }
}
