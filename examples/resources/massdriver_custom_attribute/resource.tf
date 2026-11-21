resource "massdriver_custom_attribute" "team" {
  key      = "team"
  scope    = "PROJECT"
  values   = ["platform", "data", "payments"]
  required = true
}

resource "massdriver_custom_attribute" "data_classification" {
  key    = "data_classification"
  scope  = "ENVIRONMENT"
  values = ["public", "internal", "restricted"]
}
