# A value with a label shows as a row on the instance's Metadata tab.
resource "massdriver_stash_entry" "db_hostname" {
  instance = "ecomm-prod-db"
  key      = "db/hostname"
  value    = aws_db_instance.main.address
  label    = "Database host"
}

# Runbook and product links show as buttons in the environment header.
resource "massdriver_stash_entry" "links" {
  environment = "ecomm-prod"
  key         = "md/v1/links"
  format      = "JSON"
  value = jsonencode({
    links = [
      { label = "Runbook", url = "https://wiki.example.com/ecomm" },
      { label = "Shop", url = "https://shop.example.com" },
    ]
  })
}

# A bundle records what runs on its instance. Inside a bundle deployment, the
# provider signs in with the deployment's token.
resource "massdriver_stash_entry" "release" {
  instance = "ecomm-prod-api"
  key      = "md/v1/release"
  format   = "JSON"
  value = jsonencode({
    version = var.app_version
    image   = "${var.image_repository}:${var.image_tag}"
  })
}

# A secret that a later deployment reads. Massdriver encrypts it, and
# Terraform hides it in plan output.
resource "massdriver_stash_entry" "kubeconfig" {
  environment     = "ecomm-prod"
  key             = "kubeconfig"
  sensitive_value = module.cluster.kubeconfig
}
