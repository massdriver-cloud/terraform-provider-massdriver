resource "massdriver_organization_settings" "this" {
  grant_new_bundles_to_all_projects = true

  naming_convention = "{{attrs.team}}-{{instance.id}}"

  default_tags = [
    massdriver_custom_attribute.team.key,
  ]
}
