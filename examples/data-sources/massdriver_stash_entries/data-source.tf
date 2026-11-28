# List the database entries in an instance's stash.
data "massdriver_stash_entries" "db" {
  instance   = "ecomm-prod-db"
  key_prefix = "db/"
}

locals {
  db_settings = { for e in data.massdriver_stash_entries.db.entries : e.key => e.value }
}
