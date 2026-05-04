package massdriver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"terraform-provider-massdriver/internal/api"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGroupPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "An ABAC policy attached to a group. Each policy grants (`ALLOW`) or blocks (`DENY`) one or more actions on resources whose attributes match the conditions. `DENY` always wins over `ALLOW`.",

		CreateContext: resourceGroupPolicyCreate,
		ReadContext:   resourceGroupPolicyRead,
		UpdateContext: resourceGroupPolicyUpdate,
		DeleteContext: resourceGroupPolicyDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceGroupPolicyImport,
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
					string(api.PolicyEffectAllow),
					string(api.PolicyEffectDeny),
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
	client := meta.(*ProviderClient).Client

	policy, err := api.CreateGroupPolicy(ctx, client, d.Get("group_id").(string), api.CreateGroupPolicyInput{
		Actions:    actionsFromConfig(d.Get("actions")),
		Conditions: api.EncodeConditions(d.Get("conditions").(string)),
		Effect:     api.PolicyEffect(d.Get("effect").(string)),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(policy.ID)
	return resourceGroupPolicyRead(ctx, d, meta)
}

func resourceGroupPolicyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	policy, err := api.GetGroupPolicy(ctx, client, d.Get("group_id").(string), d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if policy == nil {
		// Policy was deleted out of band — clear state so terraform re-creates on next apply.
		d.SetId("")
		return nil
	}

	d.Set("effect", policy.Effect)
	d.Set("actions", policy.Actions)
	d.Set("conditions", policy.Conditions)
	d.Set("group_id", policy.GroupID)
	return nil
}

func resourceGroupPolicyUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	_, err := api.UpdatePolicy(ctx, client, d.Id(), api.UpdatePolicyInput{
		Actions:    actionsFromConfig(d.Get("actions")),
		Conditions: api.EncodeConditions(d.Get("conditions").(string)),
		Effect:     api.PolicyEffect(d.Get("effect").(string)),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceGroupPolicyRead(ctx, d, meta)
}

func resourceGroupPolicyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeletePolicy(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// actionsFromConfig extracts the user's `actions` set into a sorted []string.
// Sorting keeps the wire payload deterministic across applies regardless of the
// (unordered) Set traversal order, which prevents spurious diffs.
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

// resourceGroupPolicyImport accepts `<group_id>/<policy_id>` because Read needs
// both — the schema has no top-level policy(id) query.
func resourceGroupPolicyImport(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("import ID must be in `<group_id>/<policy_id>` form, got %q", d.Id())
	}
	d.Set("group_id", parts[0])
	d.SetId(parts[1])
	return []*schema.ResourceData{d}, nil
}
