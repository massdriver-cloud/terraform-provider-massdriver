package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceComponentLinkCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"linkComponents": {
			"data": map[string]any{
				"linkComponents": map[string]any{
					"result": map[string]any{
						"id":            "link-123",
						"fromField":     "authentication",
						"toField":       "database",
						"fromComponent": map[string]any{"id": "ecomm-db", "name": "Database"},
						"toComponent":   map[string]any{"id": "ecomm-app", "name": "App"},
					},
					"successful": true,
				},
			},
		},
		"listLinks": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"links": map[string]any{
							"items": []map[string]any{
								{
									"id":            "link-123",
									"fromField":     "authentication",
									"toField":       "database",
									"fromComponent": map[string]any{"id": "ecomm-db"},
									"toComponent":   map[string]any{"id": "ecomm-app"},
								},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"project_id":        "ecomm",
		"from_component_id": "ecomm-db",
		"from_field":        "authentication",
		"from_version":      "~1.0",
		"to_component_id":   "ecomm-app",
		"to_field":          "database",
		"to_version":        "~2.0",
	})

	diags := resourceComponentLinkCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "link-123" {
		t.Errorf("got id %q, want link-123", rd.Id())
	}

	linkReq := rec.FindRequest("linkComponents")
	if linkReq == nil {
		t.Fatal("linkComponents was not called")
	}
	vars := gqlmock.Variables(linkReq)
	input, _ := vars["input"].(map[string]any)
	wantInput := map[string]any{
		"fromComponentId": "ecomm-db",
		"fromField":       "authentication",
		"fromVersion":     "~1.0",
		"toComponentId":   "ecomm-app",
		"toField":         "database",
		"toVersion":       "~2.0",
	}
	for k, want := range wantInput {
		if input[k] != want {
			t.Errorf("input.%s: got %v, want %v", k, input[k], want)
		}
	}
}

func TestResourceComponentLinkReadDropsMissingLink(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"listLinks": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"links": map[string]any{
							"items": []map[string]any{
								// A different link between the same components — must NOT match by ID.
								{
									"id":        "different-link",
									"fromField": "auth",
									"toField":   "db",
								},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"project_id":        "ecomm",
		"from_component_id": "ecomm-db",
		"to_component_id":   "ecomm-app",
	})
	rd.SetId("link-123")

	diags := resourceComponentLinkRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared when link is gone, got %q", rd.Id())
	}
}

func TestResourceComponentLinkReadFiltersByEndpoints(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listLinks": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"links": map[string]any{
							"items": []map[string]any{
								{
									"id":        "link-123",
									"fromField": "authentication",
									"toField":   "database",
								},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"project_id":        "ecomm",
		"from_component_id": "ecomm-db",
		"to_component_id":   "ecomm-app",
	})
	rd.SetId("link-123")

	if diags := resourceComponentLinkRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	listReq := rec.FindRequest("listLinks")
	if listReq == nil {
		t.Fatal("listLinks was not called")
	}
	vars := gqlmock.Variables(listReq)
	if vars["projectId"] != "ecomm" {
		t.Errorf("got projectId %v, want ecomm", vars["projectId"])
	}
	filter, _ := vars["filter"].(map[string]any)
	if filter == nil {
		t.Fatal("expected listLinks filter, got none")
	}
	if from, _ := filter["fromComponentId"].(map[string]any); from["eq"] != "ecomm-db" {
		t.Errorf("filter.fromComponentId.eq: got %v, want ecomm-db", from["eq"])
	}
	if to, _ := filter["toComponentId"].(map[string]any); to["eq"] != "ecomm-app" {
		t.Errorf("filter.toComponentId.eq: got %v, want ecomm-app", to["eq"])
	}
}

func TestResourceComponentLinkDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"unlinkComponents": {
			"data": map[string]any{
				"unlinkComponents": map[string]any{
					"result":     map[string]any{"id": "link-123"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-123")

	if diags := resourceComponentLinkDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("unlinkComponents")); vars["id"] != "link-123" {
		t.Errorf("got id %v, want link-123", vars["id"])
	}
}

func TestResourceComponentLinkSchema(t *testing.T) {
	r := resourceComponentLink()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if r.UpdateContext != nil {
		t.Error("UpdateContext should be nil — links have no update mutation")
	}
	for _, field := range []string{"project_id", "from_component_id", "from_field", "from_version", "to_component_id", "to_field", "to_version"} {
		s := r.Schema[field]
		if s == nil || !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew", field)
		}
	}
}
