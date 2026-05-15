# A deployment context inside a project (e.g., prod, staging). Components
# inside the project blueprint are materialized once per environment.

resource "massdriver_environment" "prod" {
  project_id  = massdriver_project.ecommerce.id
  identifier  = "prod"
  name        = "Production"
  description = "Live customer traffic."

  # `targetSLA` is required for environments in many orgs (set under the
  # organization's custom-attribute schema in the Massdriver console).
  attributes = {
    targetSLA = "99.9"
  }
}
