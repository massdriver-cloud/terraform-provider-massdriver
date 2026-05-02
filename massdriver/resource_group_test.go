package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceGroupCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createGroup": {
			"data": map[string]any{
				"createGroup": map[string]any{
					"result": map[string]any{
						"id":          "group-1",
						"name":        "Platform Engineering",
						"description": "Owns the shared platform",
						"role":        "CUSTOM",
					},
					"successful": true,
				},
			},
		},
		"getGroup": {
			"data": map[string]any{
				"group": map[string]any{
					"id":          "group-1",
					"name":        "Platform Engineering",
					"description": "Owns the shared platform",
					"role":        "CUSTOM",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name":        "Platform Engineering",
		"description": "Owns the shared platform",
	})

	if diags := resourceGroupCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "group-1" {
		t.Errorf("got id %q, want group-1", rd.Id())
	}
	if rd.Get("role").(string) != "CUSTOM" {
		t.Errorf("got role %q, want CUSTOM (server-assigned)", rd.Get("role"))
	}

	createReq := rec.FindRequest("createGroup")
	if createReq == nil {
		t.Fatal("createGroup was not called")
	}
	vars := gqlmock.Variables(createReq)
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Platform Engineering" {
		t.Errorf("got input.name %v, want Platform Engineering", input["name"])
	}
	if input["description"] != "Owns the shared platform" {
		t.Errorf("got input.description %v", input["description"])
	}
	// role is server-assigned, never sent on create.
	if _, present := input["role"]; present {
		t.Errorf("role should not appear in createGroup input, got %v", input["role"])
	}
}

func TestResourceGroupCreateOmitsUnsetDescription(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createGroup": {
			"data": map[string]any{
				"createGroup": map[string]any{
					"result":     map[string]any{"id": "group-2", "name": "Just A Name", "role": "CUSTOM"},
					"successful": true,
				},
			},
		},
		"getGroup": {
			"data": map[string]any{
				"group": map[string]any{"id": "group-2", "name": "Just A Name", "role": "CUSTOM"},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name": "Just A Name",
	})

	if diags := resourceGroupCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createGroup"))["input"].(map[string]any)
	if _, present := input["description"]; present {
		t.Errorf("description should be omitted when unset, got %v", input["description"])
	}
}

func TestResourceGroupCreatePropagatesAPIFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"createGroup": {
			"data": map[string]any{
				"createGroup": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "validation", "field": "name", "message": "name has already been taken"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name": "Admins",
	})

	diags := resourceGroupCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from failed mutation, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceGroupRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getGroup": {
			"data": map[string]any{
				"group": map[string]any{
					"id":          "group-1",
					"name":        "Platform Engineering",
					"description": "from server",
					"role":        "CUSTOM",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("group-1")

	if diags := resourceGroupRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "Platform Engineering" {
		t.Errorf("got name %q, want Platform Engineering", rd.Get("name"))
	}
	if rd.Get("description").(string) != "from server" {
		t.Errorf("got description %q", rd.Get("description"))
	}
	if rd.Get("role").(string) != "CUSTOM" {
		t.Errorf("got role %q", rd.Get("role"))
	}
}

func TestResourceGroupUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateGroup": {
			"data": map[string]any{
				"updateGroup": map[string]any{
					"result": map[string]any{
						"id":          "group-1",
						"name":        "Renamed",
						"description": "updated",
						"role":        "CUSTOM",
					},
					"successful": true,
				},
			},
		},
		"getGroup": {
			"data": map[string]any{
				"group": map[string]any{
					"id":          "group-1",
					"name":        "Renamed",
					"description": "updated",
					"role":        "CUSTOM",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("group-1")

	if diags := resourceGroupUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateGroup")
	if updateReq == nil {
		t.Fatal("updateGroup was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "group-1" {
		t.Errorf("got id %v, want group-1", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Renamed" || input["description"] != "updated" {
		t.Errorf("got input %v, want name=Renamed description=updated", input)
	}
}

func TestResourceGroupDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteGroup": {
			"data": map[string]any{
				"deleteGroup": map[string]any{
					"result":     map[string]any{"id": "group-1", "name": "Platform Engineering"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("group-1")

	if diags := resourceGroupDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deleteGroup")); vars["id"] != "group-1" {
		t.Errorf("got id %v, want group-1", vars["id"])
	}
}

// Built-in groups can't be deleted server-side; the mutation comes back with
// successful=false. The resource's Delete must surface that as a diag error so
// terraform marks the destroy as failed instead of silently dropping the resource.
func TestResourceGroupDeletePropagatesBuiltInRejection(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"deleteGroup": {
			"data": map[string]any{
				"deleteGroup": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "forbidden", "field": "id", "message": "built-in groups cannot be deleted"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("group-admins")

	diags := resourceGroupDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error rejecting built-in group deletion")
	}
	if rd.Id() != "group-admins" {
		t.Errorf("ID should not be cleared on failed delete, got %q", rd.Id())
	}
}

// Description is Optional+Computed: when the user omits it from config, drift
// from a console edit must NOT show up in plan. This is the same drift-ignore
// pattern as project/environment/component.
func TestResourceGroupIgnoresDescriptionDriftWhenConfigUnset(t *testing.T) {
	r := resourceGroup()

	state := &terraform.InstanceState{
		ID: "group-1",
		Attributes: map[string]string{
			"id":          "group-1",
			"name":        "Platform Engineering",
			"description": "added in the console",
			"role":        "CUSTOM",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"name": "Platform Engineering",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff != nil && !diff.Empty() {
		if attr := diff.Attributes["description"]; attr != nil {
			t.Errorf("expected no diff on description when config omits it; got %+v", attr)
		}
	}
}

func TestResourceGroupSchema(t *testing.T) {
	r := resourceGroup()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if name := r.Schema["name"]; name == nil || !name.Required {
		t.Error("name should be Required")
	}
	if desc := r.Schema["description"]; desc == nil || desc.Required || !desc.Optional || !desc.Computed {
		t.Error("description should be Optional+Computed")
	}
	if role := r.Schema["role"]; role == nil || !role.Computed || role.Optional || role.Required {
		t.Error("role should be Computed-only (server-assigned)")
	}
	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}
