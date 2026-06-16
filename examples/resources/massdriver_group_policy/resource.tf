# An ABAC policy attached to a group. Each policy grants (ALLOW) or
# blocks (DENY) one or more actions on resources whose attributes match
# the conditions. Within one policy, conditions AND together; across
# policies on the same group, they OR together. DENY always wins over
# ALLOW.

# Specific-conditions ALLOW: members can view projects tagged with the
# eng team.
resource "massdriver_group_policy" "view_eng_projects" {
  group_id   = massdriver_group.sres.id
  effect     = "ALLOW"
  actions    = ["project:view"]
  conditions = jsonencode({ team = ["eng"] })
}

# Wildcard DENY ("*"): blocks the listed actions across every entity
# regardless of attributes. Useful for hard guardrails.
resource "massdriver_group_policy" "deny_destructive" {
  group_id   = massdriver_group.sres.id
  effect     = "DENY"
  actions    = ["project:delete", "instance:deploy"]
  conditions = "*"
}
