package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/policies"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakePolicies struct {
	getResp, createResp, updateResp, deleteResp *policies.Policy
	getErr, createErr, updateErr, deleteErr     error

	getID         string
	createGroupID string
	createInput   policies.CreatePolicyInput
	updateID      string
	updateInput   policies.UpdatePolicyInput
	deleteID      string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakePolicies) Get(_ context.Context, id string) (*policies.Policy, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakePolicies) Create(_ context.Context, groupID string, input policies.CreatePolicyInput) (*policies.Policy, error) {
	f.createGroupID = groupID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakePolicies) Update(_ context.Context, id string, input policies.UpdatePolicyInput) (*policies.Policy, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakePolicies) Delete(_ context.Context, id string) (*policies.Policy, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceGroupPolicyCreate(t *testing.T) {
	resp := &policies.Policy{
		ID:         "policy-1",
		Effect:     string(policies.EffectAllow),
		Actions:    []string{"project:view"},
		Conditions: policies.PolicyConditions{"team": {"eng"}},
		Group:      &types.Group{ID: "group-1"},
	}
	fake := &fakePolicies{createResp: resp, getResp: resp}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view"},
		"conditions": `{"team":["eng"]}`,
	})

	if diags := resourceGroupPolicyCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "policy-1" {
		t.Errorf("got id %q, want policy-1", rd.Id())
	}
	if fake.createGroupID != "group-1" {
		t.Errorf("got Create groupID %q, want group-1", fake.createGroupID)
	}
	in := fake.createInput
	if in.Effect != policies.EffectAllow {
		t.Errorf("got Effect %q, want ALLOW", in.Effect)
	}
	if len(in.Actions) != 1 || in.Actions[0] != "project:view" {
		t.Errorf("got Actions %v, want [project:view]", in.Actions)
	}
	if got := in.Conditions["team"]; len(got) != 1 || got[0] != "eng" {
		t.Errorf("got Conditions[team] %v, want [eng]", got)
	}
}

// `"*"` is the whole-policy wildcard — the SDK takes a nil map for that.
// Make sure the resource maps the literal correctly.
func TestResourceGroupPolicyCreateWildcardConditions(t *testing.T) {
	resp := &policies.Policy{
		ID:         "policy-2",
		Effect:     string(policies.EffectDeny),
		Actions:    []string{"project:delete", "instance:deploy"},
		Conditions: nil, // wildcard
		Group:      &types.Group{ID: "group-1"},
	}
	fake := &fakePolicies{createResp: resp, getResp: resp}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "DENY",
		"actions":    []any{"project:delete", "instance:deploy"},
		"conditions": "*",
	})

	if diags := resourceGroupPolicyCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.createInput.Conditions != nil {
		t.Errorf("got Conditions %v, want nil (wildcard sentinel)", fake.createInput.Conditions)
	}
	// Actions set should be sorted for deterministic wire payload.
	if got := fake.createInput.Actions; len(got) != 2 || got[0] != "instance:deploy" || got[1] != "project:delete" {
		t.Errorf("Actions should be sorted; got %v", got)
	}
}

// Invalid JSON in conditions must error before the API call.
func TestResourceGroupPolicyCreateRejectsInvalidConditionsJSON(t *testing.T) {
	fake := &fakePolicies{}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view"},
		"conditions": `not json`,
	})

	diags := resourceGroupPolicyCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected JSON parse error, got none")
	}
	if fake.createCalls != 0 {
		t.Errorf("Create should not fire on parse error; got %d calls", fake.createCalls)
	}
}

func TestResourceGroupPolicyCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakePolicies{createErr: fmt.Errorf("create policy on group group-1: invalid action 'bogus:verb'")}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"bogus:verb"},
		"conditions": "*",
	})

	diags := resourceGroupPolicyCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "invalid action") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
}

func TestResourceGroupPolicyRead(t *testing.T) {
	pc := &ProviderClient{Policies: &fakePolicies{
		getResp: &policies.Policy{
			ID:         "policy-1",
			Effect:     string(policies.EffectAllow),
			Actions:    []string{"project:view"},
			Conditions: policies.PolicyConditions{"team": {"eng"}},
			Group:      &types.Group{ID: "group-1"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("group_id").(string) != "group-1" {
		t.Errorf("got group_id %q, want group-1 (populated from Policy.Group)", rd.Get("group_id"))
	}
	if rd.Get("effect").(string) != "ALLOW" {
		t.Errorf("got effect %q", rd.Get("effect"))
	}
	if got := rd.Get("conditions").(string); got != `{"team":["eng"]}` {
		t.Errorf("got conditions %q, want plain JSON object (not double-encoded)", got)
	}
}

// A whole-policy-wildcard policy round-trips back to the literal "*" so the
// user's HCL doesn't manufacture drift on the next plan.
func TestResourceGroupPolicyReadEncodesWildcard(t *testing.T) {
	pc := &ProviderClient{Policies: &fakePolicies{
		getResp: &policies.Policy{
			ID:         "policy-w",
			Effect:     string(policies.EffectDeny),
			Actions:    []string{"project:delete"},
			Conditions: nil,
			Group:      &types.Group{ID: "group-1"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("policy-w")

	if diags := resourceGroupPolicyRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := rd.Get("conditions").(string); got != "*" {
		t.Errorf("nil conditions should encode to `*`; got %q", got)
	}
}

func TestResourceGroupPolicyReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Policies: &fakePolicies{
		getErr: fmt.Errorf("get policy gone: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceGroupPolicyRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceGroupPolicyUpdate(t *testing.T) {
	resp := &policies.Policy{
		ID:         "policy-1",
		Effect:     string(policies.EffectAllow),
		Actions:    []string{"project:view", "project:edit"},
		Conditions: policies.PolicyConditions{"team": {"eng", "ops"}},
		Group:      &types.Group{ID: "group-1"},
	}
	fake := &fakePolicies{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view", "project:edit"},
		"conditions": `{"team":["eng","ops"]}`,
	})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "policy-1" {
		t.Errorf("got updateID %q, want policy-1", fake.updateID)
	}
	in := fake.updateInput
	if in.Conditions == nil {
		t.Fatal("Update should pass a non-nil *PolicyConditions (HCL field is Required)")
	}
	if got := (*in.Conditions)["team"]; len(got) != 2 {
		t.Errorf("got Conditions[team] %v, want 2 elements", got)
	}
}

func TestResourceGroupPolicyDelete(t *testing.T) {
	fake := &fakePolicies{}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "policy-1" {
		t.Errorf("got deleteID %q, want policy-1", fake.deleteID)
	}
}

func TestResourceGroupPolicyDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakePolicies{deleteErr: fmt.Errorf("delete policy: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Policies: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceGroupPolicyDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceGroupPolicySchema(t *testing.T) {
	r := resourceGroupPolicy()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if gid := r.Schema["group_id"]; gid == nil || !gid.Required || !gid.ForceNew {
		t.Error("group_id should be Required+ForceNew")
	}
	if eff := r.Schema["effect"]; eff == nil || !eff.Required || eff.ValidateFunc == nil {
		t.Error("effect should be Required and have a ValidateFunc restricting to ALLOW/DENY")
	}
	if act := r.Schema["actions"]; act == nil || !act.Required || act.Type != schema.TypeSet || act.MinItems != 1 {
		t.Errorf("actions should be Required TypeSet with MinItems=1; got %+v", act)
	}
	if cond := r.Schema["conditions"]; cond == nil || !cond.Required {
		t.Error("conditions should be Required")
	}
}

func TestResourceGroupPolicyEffectValidation(t *testing.T) {
	effect := resourceGroupPolicy().Schema["effect"]
	if effect.ValidateFunc == nil {
		t.Fatal("effect has no ValidateFunc")
	}
	cases := map[string]bool{
		"ALLOW":      true,
		"DENY":       true,
		"allow":      false, // case-sensitive per the SDK constants
		"GRANT":      false,
		"":           false,
		"ALLOW,DENY": false,
	}
	for value, valid := range cases {
		t.Run(value, func(t *testing.T) {
			_, errs := effect.ValidateFunc(value, "effect")
			if valid && len(errs) > 0 {
				t.Errorf("expected %q to be valid, got %v", value, errs)
			}
			if !valid && len(errs) == 0 {
				t.Errorf("expected %q to be rejected, got no errors", value)
			}
		})
	}
}

// The state encoder bypasses PolicyConditions.MarshalJSON to keep state in a
// plain JSON-object form. If it instead emitted the SDK's double-encoded
// wire form (`"{\"team\":[\"eng\"]}"`), the user's `jsonencode(...)` would
// produce a different string and every plan would show drift.
func TestEncodePolicyConditionsProducesPlainJSON(t *testing.T) {
	got := encodePolicyConditions(policies.PolicyConditions{"team": {"eng"}})
	want := `{"team":["eng"]}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
