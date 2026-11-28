# Read a value that the database bundle wrote to its instance.
data "massdriver_stash_entry" "db_hostname" {
  instance = "ecomm-prod-db"
  key      = "db/hostname"
}

# Read a JSON value as an object.
data "massdriver_stash_entry" "release" {
  instance = "ecomm-prod-api"
  key      = "md/v1/release"
}

locals {
  db_hostname     = data.massdriver_stash_entry.db_hostname.value
  running_version = jsondecode(data.massdriver_stash_entry.release.data).version
}
