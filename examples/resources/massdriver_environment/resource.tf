resource "massdriver_project" "example" {
  name        = "My Application"
  slug        = "my-app"
  description = "Example application"
}

resource "massdriver_environment" "production" {
  project_id  = massdriver_project.example.id
  name        = "Production"
  slug        = "prod"
  description = "Production environment"
}

resource "massdriver_environment" "staging" {
  project_id  = massdriver_project.example.id
  name        = "Staging"
  slug        = "staging"
  description = "Staging environment for testing"
}
