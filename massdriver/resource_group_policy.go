package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/policies"
)

// policiesAPI is the slice of *policies.Service this resource calls.
type policiesAPI interface {
	Get(ctx context.Context, policyID string) (*policies.Policy, error)
	Create(ctx context.Context, groupID string, input policies.CreatePolicyInput) (*policies.Policy, error)
	Update(ctx context.Context, policyID string, input policies.UpdatePolicyInput) (*policies.Policy, error)
	Delete(ctx context.Context, policyID string) (*policies.Policy, error)
}

var _ policiesAPI = (*policies.Service)(nil)

func resourceGroupPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "An ABAC policy attached to a group. Each policy grants (`ALLOW`) or blocks (`DENY`) one or more actions on resources whose attributes match the conditions. `DENY` always wins over `ALLOW`.",

		CreateContext: resourceGroupPolicyCreate,
		ReadContext:   resourceGroupPolicyRead,
		UpdateContext: resourceGroupPolicyUpdate,
		DeleteContext: resourceGroupPolicyDelete,

		// `policies.Get` returns the parent group, so the user only needs to
		// supply the policy ID at import time; Read populates group_id.
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"group_id": {
				Description: "ID of the group this policy is attached to. Immutable — re-targeting requires destroy + recreate.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"effect": {
				Description: "Whether this policy grants (`ALLOW`) or blocks (`DENY`) the actions.",
				Type:        schema.TypeString,
				Required:    true,
				ValidateFunc: validation.StringInSlice([]string{
					string(policies.EffectAllow),
					string(policies.EffectDeny),
				}, false),
			},
			"actions": {
				Description: "One or more actions this policy applies to, each in `{entity}:{verb}` form (e.g., `project:view`, `instance:deploy`). Conditions apply to every action; entities that don't support a given condition simply never match.",
				Type:        schema.TypeSet,
				Required:    true,
				MinItems:    1,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"conditions": {
				Description: "Either the literal `\"*\"` (wildcard — matches every resource of the action's entity) or a JSON-encoded object of attribute conditions (e.g., `{\"team\":[\"eng\"]}`). Within one policy, conditions AND together; across policies on the same group, they OR together.",
				Type:        schema.TypeString,
				Required:    true,
			},
		},
	}
}

func resourceGroupPolicyCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	conditions, err := decodePolicyConditions(d.Get("conditions").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	policy, err := pc.Policies.Create(ctx, d.Get("group_id").(string), policies.CreatePolicyInput{
		Effect:     policies.Effect(d.Get("effect").(string)),
		Actions:    actionsFromConfig(d.Get("actions")),
		Conditions: conditions,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(policy.ID)
	return resourceGroupPolicyRead(ctx, d, meta)
}

func resourceGroupPolicyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	policy, err := pc.Policies.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("effect", policy.Effect)
	d.Set("actions", policy.Actions)
	d.Set("conditions", encodePolicyConditions(policy.Conditions))
	if policy.Group != nil {
		d.Set("group_id", policy.Group.ID)
	}
	return nil
}

func resourceGroupPolicyUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	conditions, err := decodePolicyConditions(d.Get("conditions").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	// UpdatePolicyInput.Conditions is `*PolicyConditions` so the SDK can
	// distinguish "leave unchanged" (nil pointer) from "set to wildcard"
	// (non-nil pointer to nil map). The provider always sends conditions
	// because the HCL field is Required, so we always pass &conditions.
	if _, err := pc.Policies.Update(ctx, d.Id(), policies.UpdatePolicyInput{
		Effect:     policies.Effect(d.Get("effect").(string)),
		Actions:    actionsFromConfig(d.Get("actions")),
		Conditions: &conditions,
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceGroupPolicyRead(ctx, d, meta)
}

func resourceGroupPolicyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Policies.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// actionsFromConfig extracts the user's `actions` set into a sorted []string.
// Sorting keeps the wire payload deterministic regardless of Set traversal
// order, preventing spurious diffs across applies.
func actionsFromConfig(raw any) []string {
	set, ok := raw.(*schema.Set)
	if !ok {
		return nil
	}
	items := set.List()
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// decodePolicyConditions parses the user's HCL `conditions` field into the
// SDK's PolicyConditions map. The literal "*" maps to a nil map (whole-policy
// wildcard); any other value must be a JSON object of `{key: [values...]}`
// form.
func decodePolicyConditions(s string) (policies.PolicyConditions, error) {
	if s == "*" {
		return nil, nil
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("invalid JSON in conditions: %w", err)
	}
	return policies.PolicyConditions(m), nil
}

// encodePolicyConditions serializes the policy's conditions back to the HCL
// form the user provided: `"*"` for wildcard, plain JSON object otherwise.
// As of SDK v0.2 PolicyConditions.MarshalJSON produces the bare object form;
// the wire-form double-encoding moved into gql/scalars and is invoked by
// genqlient transparently, so we can json.Marshal directly here.
func encodePolicyConditions(c policies.PolicyConditions) string {
	if c == nil {
		return "*"
	}
	b, _ := json.Marshal(c)
	return string(b)
}
