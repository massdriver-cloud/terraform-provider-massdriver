terraform {
  required_providers {
    massdriver = {
      source  = "local/massdriver/massdriver"
      version = "0.0.1"
    }
  }
}

provider "massdriver" {}

# Project
resource "massdriver_project" "ecommerce" {
  name        = "E-Commerce Platform (Terraform Provider Test)"
  slug        = "tfpecommerce"
  description = "Main e-commerce application managed by Terraform"
}

# Environments
resource "massdriver_environment" "production" {
  project_id  = massdriver_project.ecommerce.id
  name        = "Production"
  slug        = "prod"
  description = "Production environment"
}

resource "massdriver_environment" "staging" {
  project_id  = massdriver_project.ecommerce.id
  name        = "Staging"
  slug        = "staging"
  description = "Staging environment for testing"
}

# Outputs
output "project_id" {
  value = massdriver_project.ecommerce.id
}

output "project_slug" {
  value = massdriver_project.ecommerce.slug
}

output "production_env_id" {
  value = massdriver_environment.production.id
}

output "staging_env_id" {
  value = massdriver_environment.staging.id
}
