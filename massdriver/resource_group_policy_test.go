package massdriver

import (
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceGroupPolicyCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createGroupPolicy": {
			"data": map[string]any{
				"createGroupPolicy": map[string]any{
					"result": map[string]any{
						"id":         "policy-1",
						"effect":     "ALLOW",
						"actions":    []string{"project:view"},
						"conditions": `{"team":["eng"]}`,
						"group":      map[string]any{"id": "group-1"},
					},
					"successful": true,
				},
			},
		},
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":         "policy-1",
								"effect":     "ALLOW",
								"actions":    []string{"project:view"},
								"conditions": `{"team":["eng"]}`,
								"group":      map[string]any{"id": "group-1"},
							},
						},
					},
				},
			},
		},
	})

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

	createReq := rec.FindRequest("createGroupPolicy")
	if createReq == nil {
		t.Fatal("createGroupPolicy was not called")
	}
	vars := gqlmock.Variables(createReq)
	if vars["groupId"] != "group-1" {
		t.Errorf("got groupId %v, want group-1", vars["groupId"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["effect"] != "ALLOW" {
		t.Errorf("got input.effect %v, want ALLOW", input["effect"])
	}
	actions, _ := input["actions"].([]any)
	if len(actions) != 1 || actions[0] != "project:view" {
		t.Errorf("got input.actions %v, want [project:view]", actions)
	}
	if input["conditions"] != `{"team":["eng"]}` {
		t.Errorf("got input.conditions %v, want JSON-encoded string {\"team\":[\"eng\"]}", input["conditions"])
	}
}

// Multiple actions get serialized as a JSON array. Sorting keeps the wire
// payload deterministic regardless of the (unordered) Set traversal order.
func TestResourceGroupPolicyCreateMultipleActions(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createGroupPolicy": {
			"data": map[string]any{
				"createGroupPolicy": map[string]any{
					"result": map[string]any{
						"id":         "policy-1",
						"effect":     "ALLOW",
						"actions":    []string{"instance:deploy", "project:view"},
						"conditions": "*",
						"group":      map[string]any{"id": "group-1"},
					},
					"successful": true,
				},
			},
		},
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":         "policy-1",
								"effect":     "ALLOW",
								"actions":    []string{"instance:deploy", "project:view"},
								"conditions": "*",
								"group":      map[string]any{"id": "group-1"},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view", "instance:deploy"}, // intentionally out of order
		"conditions": "*",
	})

	if diags := resourceGroupPolicyCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createGroupPolicy"))["input"].(map[string]any)
	got, _ := input["actions"].([]any)
	gotStrs := make([]string, len(got))
	for i, v := range got {
		gotStrs[i], _ = v.(string)
	}
	want := []string{"instance:deploy", "project:view"}
	if len(gotStrs) != len(want) {
		t.Fatalf("got %v, want %v", gotStrs, want)
	}
	for i, s := range want {
		if gotStrs[i] != s {
			t.Errorf("actions[%d] = %q, want %q (sorted, deterministic)", i, gotStrs[i], s)
		}
	}
	// Sanity: confirm sort.StringsAreSorted holds, since determinism is the load-bearing property.
	if !sort.StringsAreSorted(gotStrs) {
		t.Errorf("actions wire payload should be sorted, got %v", gotStrs)
	}
}

// `"*"` is the literal wildcard — it must reach the API as the bare string,
// not be JSON-double-encoded into something like `"\"*\""`.
func TestResourceGroupPolicyCreateWildcard(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createGroupPolicy": {
			"data": map[string]any{
				"createGroupPolicy": map[string]any{
					"result": map[string]any{
						"id":         "policy-wild",
						"effect":     "DENY",
						"actions":    []string{"project:delete"},
						"conditions": "*",
						"group":      map[string]any{"id": "group-1"},
					},
					"successful": true,
				},
			},
		},
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":         "policy-wild",
								"effect":     "DENY",
								"actions":    []string{"project:delete"},
								"conditions": "*",
								"group":      map[string]any{"id": "group-1"},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "DENY",
		"actions":    []any{"project:delete"},
		"conditions": "*",
	})

	if diags := resourceGroupPolicyCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createGroupPolicy"))["input"].(map[string]any)
	if input["conditions"] != "*" {
		t.Errorf("got input.conditions %v, want literal *", input["conditions"])
	}
}

func TestResourceGroupPolicyCreatePropagatesAPIFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"createGroupPolicy": {
			"data": map[string]any{
				"createGroupPolicy": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "validation", "field": "actions", "message": "unknown action: foo:bar"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"foo:bar"},
		"conditions": "*",
	})

	diags := resourceGroupPolicyCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceGroupPolicyRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":         "policy-1",
								"effect":     "ALLOW",
								"actions":    []string{"project:view", "instance:deploy"},
								"conditions": `{"team":["eng"]}`,
								"group":      map[string]any{"id": "group-1"},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view"},
		"conditions": `{"team":["eng"]}`,
	})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("effect").(string) != "ALLOW" {
		t.Errorf("got effect %q, wanted ALLOW", rd.Get("effect"))
	}
	got := rd.Get("actions").(*schema.Set).List()
	gotStrs := make([]string, len(got))
	for i, v := range got {
		gotStrs[i], _ = v.(string)
	}
	sort.Strings(gotStrs)
	want := []string{"instance:deploy", "project:view"}
	if len(gotStrs) != len(want) {
		t.Fatalf("got %d actions in state, want 2", len(gotStrs))
	}
	for i, s := range want {
		if gotStrs[i] != s {
			t.Errorf("actions[%d] = %q, want %q", i, gotStrs[i], s)
		}
	}
}

// Out-of-band deletion: API returns no matching policy → Read clears state so
// terraform re-creates on the next apply.
func TestResourceGroupPolicyReadClearsWhenMissing(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":      "different-policy",
								"effect":  "ALLOW",
								"actions": []string{"project:view"},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "ALLOW",
		"actions":    []any{"project:view"},
		"conditions": "*",
	})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("expected ID cleared when policy is missing, got %q", rd.Id())
	}
}

func TestResourceGroupPolicyUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updatePolicy": {
			"data": map[string]any{
				"updatePolicy": map[string]any{
					"result": map[string]any{
						"id":         "policy-1",
						"effect":     "DENY",
						"actions":    []string{"project:view"},
						"conditions": "*",
						"group":      map[string]any{"id": "group-1"},
					},
					"successful": true,
				},
			},
		},
		"listGroupPolicies": {
			"data": map[string]any{
				"group": map[string]any{
					"policies": map[string]any{
						"items": []map[string]any{
							{
								"id":         "policy-1",
								"effect":     "DENY",
								"actions":    []string{"project:view"},
								"conditions": "*",
								"group":      map[string]any{"id": "group-1"},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{
		"group_id":   "group-1",
		"effect":     "DENY",
		"actions":    []any{"project:view"},
		"conditions": "*",
	})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updatePolicy")
	if updateReq == nil {
		t.Fatal("updatePolicy was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "policy-1" {
		t.Errorf("got id %v, want policy-1", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["effect"] != "DENY" {
		t.Errorf("got input.effect %v, want DENY", input["effect"])
	}
	if input["conditions"] != "*" {
		t.Errorf("got input.conditions %v, want *", input["conditions"])
	}
}

func TestResourceGroupPolicyDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deletePolicy": {
			"data": map[string]any{
				"deletePolicy": map[string]any{
					"result":     map[string]any{"id": "policy-1"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
	rd.SetId("policy-1")

	if diags := resourceGroupPolicyDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deletePolicy")); vars["id"] != "policy-1" {
		t.Errorf("got id %v, want policy-1", vars["id"])
	}
}

func TestResourceGroupPolicyImport(t *testing.T) {
	cases := []struct {
		name      string
		importID  string
		wantGroup string
		wantID    string
		wantErr   bool
	}{
		{name: "valid", importID: "group-1/policy-1", wantGroup: "group-1", wantID: "policy-1"},
		{name: "missing slash", importID: "policy-1", wantErr: true},
		{name: "empty group", importID: "/policy-1", wantErr: true},
		{name: "empty policy", importID: "group-1/", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, resourceGroupPolicy().Schema, map[string]any{})
			rd.SetId(tc.importID)
			out, err := resourceGroupPolicyImport(t.Context(), rd, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.importID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("got %d resources, want 1", len(out))
			}
			if rd.Id() != tc.wantID {
				t.Errorf("got id %q, want %q", rd.Id(), tc.wantID)
			}
			if rd.Get("group_id").(string) != tc.wantGroup {
				t.Errorf("got group_id %q, want %q", rd.Get("group_id"), tc.wantGroup)
			}
		})
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
	for _, field := range []string{"effect", "actions", "conditions"} {
		if !r.Schema[field].Required {
			t.Errorf("%s should be Required", field)
		}
	}
	// actions is a Set of strings with at least one element.
	if a := r.Schema["actions"]; a == nil || a.Type != schema.TypeSet || a.MinItems != 1 {
		t.Errorf("actions should be a TypeSet with MinItems=1, got Type=%v MinItems=%d", a.Type, a.MinItems)
	}
	if r.Schema["effect"].ValidateFunc == nil {
		t.Error("effect should have a ValidateFunc enforcing ALLOW/DENY")
	}
}

func TestResourceGroupPolicyEffectValidation(t *testing.T) {
	effect := resourceGroupPolicy().Schema["effect"]
	cases := map[string]bool{"ALLOW": true, "DENY": true, "allow": false, "deny": false, "MAYBE": false, "": false}
	for v, ok := range cases {
		_, errs := effect.ValidateFunc(v, "effect")
		if ok && len(errs) > 0 {
			t.Errorf("expected %q to validate, got errors: %v", v, errs)
		}
		if !ok && len(errs) == 0 {
			t.Errorf("expected %q to be rejected, got no errors", v)
		}
	}
}
