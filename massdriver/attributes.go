package massdriver

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

// attributesSchema returns the schema entry for the "attributes" attribute on
// project/environment/component resources. Attributes are key-value strings
// the platform uses to compute permissions and policy, so:
//
//   - The field is Required — every resource must declare its attributes block
//     even if empty (the SDK treats `attributes = {}` as "set"). The server may
//     additionally require specific keys based on the org's custom-attribute
//     schema (e.g., `team` on projects, `env` on environments); those failures
//     surface as API errors at apply time, not at plan time. The provider
//     can't validate them ahead of time because the schema is org-scoped and
//     mutable — what's required for projects in one org may not be required
//     in another, and admins can change the rules. If you see
//     `Required property X was not present` or `X Schema does not allow
//     additional properties`, check the org's custom-attribute config in the
//     Massdriver console.
//
//   - The field is NOT Computed — drift always surfaces. If someone edits an
//     attribute in the console, the next plan reverts it to what's in HCL.
//     This is intentional: attributes drive permissions, so silent drift would
//     be a security issue.
func attributesSchema(scope string) *schema.Schema {
	return &schema.Schema{
		Description: "Key-value attributes assigned to this " + scope + ". Used by the platform to compute permissions and policy. Required keys are configured per-organization in the Massdriver console — missing or unknown keys surface as API errors at apply time. Drift is always surfaced; console edits are reverted on the next apply.",
		Type:        schema.TypeMap,
		Required:    true,
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	}
}

// attributesFromConfig converts the raw map[string]any that terraform hands us
// out of d.Get("attributes") into the map[string]any shape the API expects.
// Values from a TypeMap with `Elem: TypeString` are always strings, so this is
// a shallow copy — but the ceremony keeps the resource-side code obvious.
func attributesFromConfig(raw any) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// attributesToState converts API-returned attributes (map[string]any with
// string values) into the map[string]string shape terraform's TypeMap expects
// when calling d.Set. Non-string values (shouldn't happen — the API guarantees
// string values) are coerced via fmt-less default formatting.
func attributesToState(attrs map[string]any) map[string]string {
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
