package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceResourceCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createResource": {
			"data": map[string]any{
				"createResource": map[string]any{
					"result": map[string]any{
						"id":     "res-1",
						"name":   "CI Role",
						"origin": "IMPORTED",
						"resourceType": map[string]any{
							"id":   "aws-iam-role",
							"name": "AWS IAM Role",
						},
					},
					"successful": true,
				},
			},
		},
		"getResource": {
			"data": map[string]any{
				"resource": map[string]any{
					"id":     "res-1",
					"name":   "CI Role",
					"origin": "IMPORTED",
					"resourceType": map[string]any{
						"id":   "aws-iam-role",
						"name": "AWS IAM Role",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"name":             "CI Role",
		"resource_type_id": "aws-iam-role",
		"payload":          `{"arn":"arn:aws:iam::123:role/ci"}`,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "res-1" {
		t.Errorf("got id %q, want res-1", rd.Id())
	}
	if rd.Get("resource_type_id").(string) != "aws-iam-role" {
		t.Errorf("got resource_type_id %q, want aws-iam-role", rd.Get("resource_type_id"))
	}

	createReq := rec.FindRequest("createResource")
	if createReq == nil {
		t.Fatal("createResource was not called")
	}
	vars := gqlmock.Variables(createReq)
	if vars["resourceTypeId"] != "aws-iam-role" {
		t.Errorf("got resourceTypeId %v, want aws-iam-role", vars["resourceTypeId"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "CI Role" {
		t.Errorf("got input.name %v, want CI Role", input["name"])
	}
	// Payload is sent over the wire as a JSON-encoded string (Massdriver's `JSON` scalar marshals twice).
	payloadStr, ok := input["payload"].(string)
	if !ok {
		t.Fatalf("input.payload should be a string (JSON scalar), got %T (%v)", input["payload"], input["payload"])
	}
	if payloadStr != `{"arn":"arn:aws:iam::123:role/ci"}` {
		t.Errorf("got payload %q, want %q", payloadStr, `{"arn":"arn:aws:iam::123:role/ci"}`)
	}
}

func TestResourceResourceCreateRejectsInvalidPayload(t *testing.T) {
	pc, rec := newMockProvider(nil)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"name":             "CI Role",
		"resource_type_id": "aws-iam-role",
		"payload":          `not-valid-json`,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from invalid JSON payload")
	}
	if len(rec.Requests) != 0 {
		t.Errorf("expected no API calls when payload fails to parse, got %d", len(rec.Requests))
	}
}

func TestResourceResourceCreateAcceptsEmptyPayload(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createResource": {
			"data": map[string]any{
				"createResource": map[string]any{
					"result": map[string]any{
						"id":     "res-1",
						"name":   "Connection-only",
						"origin": "IMPORTED",
						"resourceType": map[string]any{
							"id":   "external-link",
							"name": "External Link",
						},
					},
					"successful": true,
				},
			},
		},
		"getResource": {
			"data": map[string]any{
				"resource": map[string]any{
					"id":   "res-1",
					"name": "Connection-only",
					"resourceType": map[string]any{
						"id":   "external-link",
						"name": "External Link",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"name":             "Connection-only",
		"resource_type_id": "external-link",
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	createReq := rec.FindRequest("createResource")
	if createReq == nil {
		t.Fatal("createResource was not called")
	}
	// Empty/nil payloads must be omitted from the wire entirely. GraphQL rejects
	// `payload: null` for the JSON scalar even though the schema marks the field
	// optional; the scalar marshaler returns empty bytes for nil maps so genqlient's
	// `omitempty` tag drops the field.
	input, _ := gqlmock.Variables(createReq)["input"].(map[string]any)
	if _, present := input["payload"]; present {
		t.Errorf("payload should be omitted when not supplied, got %v", input["payload"])
	}
}

func TestResourceResourceRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getResource": {
			"data": map[string]any{
				"resource": map[string]any{
					"id":     "res-1",
					"name":   "Server-side Name",
					"origin": "IMPORTED",
					"resourceType": map[string]any{
						"id":   "aws-iam-role",
						"name": "AWS IAM Role",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	diags := resourceResourceRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "Server-side Name" {
		t.Errorf("got name %q, want Server-side Name", rd.Get("name"))
	}
	if rd.Get("resource_type_id").(string) != "aws-iam-role" {
		t.Errorf("got resource_type_id %q, want aws-iam-role", rd.Get("resource_type_id"))
	}
}

func TestResourceResourceUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateResource": {
			"data": map[string]any{
				"updateResource": map[string]any{
					"result": map[string]any{
						"id":     "res-1",
						"name":   "Updated",
						"origin": "IMPORTED",
					},
					"successful": true,
				},
			},
		},
		"getResource": {
			"data": map[string]any{
				"resource": map[string]any{
					"id":     "res-1",
					"name":   "Updated",
					"origin": "IMPORTED",
					"resourceType": map[string]any{
						"id":   "aws-iam-role",
						"name": "AWS IAM Role",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"name":             "Updated",
		"resource_type_id": "aws-iam-role",
		"payload":          `{"arn":"arn:aws:iam::123:role/new"}`,
	})
	rd.SetId("res-1")

	diags := resourceResourceUpdate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateResource")
	if updateReq == nil {
		t.Fatal("updateResource was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "res-1" {
		t.Errorf("got id %v, want res-1", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Updated" {
		t.Errorf("got input.name %v, want Updated", input["name"])
	}
}

func TestResourceResourceDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteResource": {
			"data": map[string]any{
				"deleteResource": map[string]any{
					"result":     map[string]any{"id": "res-1", "name": "CI Role", "origin": "IMPORTED"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	diags := resourceResourceDelete(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deleteResource")); vars["id"] != "res-1" {
		t.Errorf("got id %v, want res-1", vars["id"])
	}
}

func TestResourceResourcePropagatesDeleteFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"deleteResource": {
			"data": map[string]any{
				"deleteResource": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "conflict", "field": "id", "message": "still in use"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	diags := resourceResourceDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from delete failure")
	}
	if rd.Id() != "res-1" {
		t.Errorf("ID should not be cleared on failed delete, got %q", rd.Id())
	}
}

func TestResourceResourceSchema(t *testing.T) {
	r := resourceResource()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if rt := r.Schema["resource_type_id"]; rt == nil || !rt.Required || !rt.ForceNew {
		t.Error("resource_type_id should be Required+ForceNew")
	}
	if payload := r.Schema["payload"]; payload == nil || !payload.Sensitive {
		t.Error("payload should be Sensitive")
	}
}
