package api_test

import (
	"encoding/json"
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetGroupPolicy(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
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
						{
							"id":         "policy-2",
							"effect":     "DENY",
							"actions":    []string{"instance:deploy"},
							"conditions": "*",
							"group":      map[string]any{"id": "group-1"},
						},
					},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.GetGroupPolicy(t.Context(), &mdClient, "group-1", "policy-2")
	if err != nil {
		t.Fatal(err)
	}
	if p == nil {
		t.Fatal("expected to find policy-2, got nil")
	}
	if p.Effect != "DENY" || len(p.Actions) != 1 || p.Actions[0] != "instance:deploy" || p.Conditions != "*" {
		t.Errorf("got %+v, wanted DENY instance:deploy *", p)
	}
	if p.GroupID != "group-1" {
		t.Errorf("got groupId %q, wanted group-1", p.GroupID)
	}
}

// Returning a nil policy (rather than an error) is the contract for "not
// found" — the terraform Read uses that to clear state on drift.
func TestGetGroupPolicyReturnsNilWhenMissing(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"group": map[string]any{
				"policies": map[string]any{"items": []map[string]any{}},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.GetGroupPolicy(t.Context(), &mdClient, "group-1", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if p != nil {
		t.Errorf("expected nil for missing policy, got %+v", p)
	}
}

func TestCreateGroupPolicy(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createGroupPolicy": map[string]any{
				"result": map[string]any{
					"id":         "policy-new",
					"effect":     "ALLOW",
					"actions":    []string{"project:view"},
					"conditions": `{"team":["eng"]}`,
					"group":      map[string]any{"id": "group-1"},
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.CreateGroupPolicy(t.Context(), &mdClient, "group-1", api.CreateGroupPolicyInput{
		Actions:    []string{"project:view"},
		Conditions: json.RawMessage(`{"team":["eng"]}`),
		Effect:     api.PolicyEffectAllow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "policy-new" {
		t.Errorf("got ID %s, wanted policy-new", p.ID)
	}
	// GroupID is lifted from the nested Group field on the response.
	if p.GroupID != "group-1" {
		t.Errorf("got groupId %s, wanted group-1", p.GroupID)
	}
}

// Conditions value `"*"` (the literal wildcard) must round-trip cleanly —
// it's a different code path than a JSON-encoded conditions object.
func TestCreateGroupPolicyWildcard(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
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
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.CreateGroupPolicy(t.Context(), &mdClient, "group-1", api.CreateGroupPolicyInput{
		Actions:    []string{"project:delete"},
		Conditions: json.RawMessage(`"*"`),
		Effect:     api.PolicyEffectDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Conditions != "*" {
		t.Errorf("got conditions %q, wanted *", p.Conditions)
	}
}

func TestCreateGroupPolicyFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createGroupPolicy": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "action", "message": "unknown action: foo:bar"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.CreateGroupPolicy(t.Context(), &mdClient, "group-1", api.CreateGroupPolicyInput{
		Actions: []string{"foo:bar"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdatePolicy(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
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
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.UpdatePolicy(t.Context(), &mdClient, "policy-1", api.UpdatePolicyInput{
		Effect:     api.PolicyEffectDeny,
		Conditions: json.RawMessage(`"*"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect != "DENY" {
		t.Errorf("got effect %q, wanted DENY", p.Effect)
	}
}

func TestDeletePolicy(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deletePolicy": map[string]any{
				"result":     map[string]any{"id": "policy-1"},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	p, err := api.DeletePolicy(t.Context(), &mdClient, "policy-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "policy-1" {
		t.Errorf("got ID %s, wanted policy-1", p.ID)
	}
}

func TestDeletePolicyFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deletePolicy": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "not_found", "field": "id", "message": "policy not found"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.DeletePolicy(t.Context(), &mdClient, "policy-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
