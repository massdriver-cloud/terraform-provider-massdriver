package massdriver

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// identifierPattern matches the Massdriver identifier rules: 1–20 characters,
// lowercase letters and digits only. Identifiers compose into package IDs of
// the form `<project>-<env>-<component>`, so any non-alphanumeric character
// or uppercase letter would break that join.
var identifierPattern = regexp.MustCompile(`^[a-z0-9]{1,20}$`)

// identifierSchema returns the schema entry for the user-supplied "identifier"
// attribute on a project/environment/component. The platform combines these
// (e.g., `<project>-<env>`) to produce the resource's ID, which terraform
// exposes via the auto-managed `id` attribute.
func identifierSchema(scope string) *schema.Schema {
	return &schema.Schema{
		Description:  "Short, immutable identifier for this " + scope + ". Composed with parent identifiers to form the platform ID. Max 20 characters, lowercase alphanumeric (a-z, 0-9).",
		Type:         schema.TypeString,
		Required:     true,
		ForceNew:     true,
		ValidateFunc: validation.StringMatch(identifierPattern, "must be 1-20 lowercase alphanumeric characters (a-z, 0-9)"),
	}
}
