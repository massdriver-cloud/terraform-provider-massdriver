package api_test

import (
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetGroup(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"group": map[string]any{
				"id":          "group-uuid1",
				"name":        "Platform Engineering",
				"description": "Owns the shared platform",
				"role":        "CUSTOM",
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	group, err := api.GetGroup(t.Context(), &mdClient, "group-uuid1")
	if err != nil {
		t.Fatal(err)
	}

	if group.ID != "group-uuid1" {
		t.Errorf("got ID %s, wanted group-uuid1", group.ID)
	}
	if group.Name != "Platform Engineering" {
		t.Errorf("got name %q, wanted Platform Engineering", group.Name)
	}
	if group.Description != "Owns the shared platform" {
		t.Errorf("got description %q", group.Description)
	}
	if group.Role != "CUSTOM" {
		t.Errorf("got role %q, wanted CUSTOM", group.Role)
	}
}

// Built-in groups (organization_admin / organization_viewer) carry their own
// roles. The wrapper just surfaces whatever the server says — no special-casing.
func TestGetGroup_PreservesBuiltInRole(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"group": map[string]any{
				"id":   "group-admins",
				"name": "Admins",
				"role": "ORGANIZATION_ADMIN",
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	group, err := api.GetGroup(t.Context(), &mdClient, "group-admins")
	if err != nil {
		t.Fatal(err)
	}
	if group.Role != "ORGANIZATION_ADMIN" {
		t.Errorf("got role %q, wanted ORGANIZATION_ADMIN", group.Role)
	}
}

func TestCreateGroup(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createGroup": map[string]any{
				"result": map[string]any{
					"id":          "group-new",
					"name":        "Platform Engineering",
					"description": "Owns the shared platform",
					"role":        "CUSTOM",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	group, err := api.CreateGroup(t.Context(), &mdClient, api.CreateGroupInput{
		Name:        "Platform Engineering",
		Description: "Owns the shared platform",
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "group-new" {
		t.Errorf("got ID %s, wanted group-new", group.ID)
	}
	// New custom groups always come back with role=CUSTOM — verify the wrapper
	// surfaces the server-assigned role rather than echoing input.
	if group.Role != "CUSTOM" {
		t.Errorf("got role %q, wanted CUSTOM", group.Role)
	}
}

func TestCreateGroupFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createGroup": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "name", "message": "name has already been taken"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.CreateGroup(t.Context(), &mdClient, api.CreateGroupInput{Name: "Admins"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateGroup(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
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
	})
	mdClient := client.Client{GQLv2: gqlClient}

	group, err := api.UpdateGroup(t.Context(), &mdClient, "group-1", api.UpdateGroupInput{
		Name:        "Renamed",
		Description: "updated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "Renamed" {
		t.Errorf("got name %q, wanted Renamed", group.Name)
	}
}

func TestDeleteGroup(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteGroup": map[string]any{
				"result":     map[string]any{"id": "group-1", "name": "Platform Engineering"},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	group, err := api.DeleteGroup(t.Context(), &mdClient, "group-1")
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != "group-1" {
		t.Errorf("got ID %s, wanted group-1", group.ID)
	}
}

// Built-in groups (organization_admin/organization_viewer) can't be deleted —
// the API returns successful=false with a permission message. The wrapper must
// surface that as an error rather than silently succeeding.
func TestDeleteGroupRejectsBuiltIn(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteGroup": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "forbidden", "field": "id", "message": "built-in groups cannot be deleted"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.DeleteGroup(t.Context(), &mdClient, "group-admins")
	if err == nil {
		t.Fatal("expected error rejecting built-in group deletion")
	}
}
