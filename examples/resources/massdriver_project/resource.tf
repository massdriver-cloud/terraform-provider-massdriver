# The top-level container for related infrastructure. Owns one or more
# environments (deployment contexts) and the project's blueprint
# (components + their links).

resource "massdriver_project" "ecommerce" {
  identifier  = "ecomm"
  name        = "E-commerce Platform"
  description = "Primary customer-facing app and its dependencies."

  # Custom attributes are configured per-organization in the Massdriver console;
  # required keys vary by org. `team` is required for projects in most setups.
  attributes = {
    team = "platform"
  }
}
