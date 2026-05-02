package api_test

import (
	"testing"

	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

func TestAddComponent(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"addComponent": map[string]any{
				"result": map[string]any{
					"id":   "ecomm-db",
					"name": "Primary Database",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	comp, err := api.AddComponent(t.Context(), &mdClient, "ecomm", "aws-rds-cluster", api.AddComponentInput{
		Id:   "db",
		Name: "Primary Database",
	})
	if err != nil {
		t.Fatal(err)
	}

	if comp.ID != "ecomm-db" {
		t.Errorf("got ID %s, wanted ecomm-db", comp.ID)
	}
}

func TestAddComponentFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"addComponent": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "id", "message": "id is required"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.AddComponent(t.Context(), &mdClient, "ecomm", "aws-rds-cluster", api.AddComponentInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateComponent(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"updateComponent": map[string]any{
				"result": map[string]any{
					"id":          "ecomm-db",
					"name":        "Renamed Database",
					"description": "updated",
					"attributes":  map[string]any{"team": "platform"},
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	comp, err := api.UpdateComponent(t.Context(), &mdClient, "ecomm-db", api.UpdateComponentInput{
		Name:        "Renamed Database",
		Description: "updated",
		Attributes:  map[string]any{"team": "platform"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Name != "Renamed Database" {
		t.Errorf("got name %s, wanted Renamed Database", comp.Name)
	}
}

// updateComponent rejects an unknown attribute key against the org's
// custom-attribute schema. The wrapper must surface that as an error rather
// than silently succeeding so terraform can fail the apply.
func TestUpdateComponentFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"updateComponent": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "attributes", "message": "Schema does not allow additional properties"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.UpdateComponent(t.Context(), &mdClient, "ecomm-db", api.UpdateComponentInput{
		Name:       "x",
		Attributes: map[string]any{"unknown": "key"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveComponent(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"removeComponent": map[string]any{
				"result":     map[string]any{"id": "ecomm-db", "name": "db"},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	comp, err := api.RemoveComponent(t.Context(), &mdClient, "ecomm-db")
	if err != nil {
		t.Fatal(err)
	}
	if comp.ID != "ecomm-db" {
		t.Errorf("got %s, wanted ecomm-db", comp.ID)
	}
}

func TestLinkComponents(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"linkComponents": map[string]any{
				"result": map[string]any{
					"id":        "link-new",
					"fromField": "authentication",
					"toField":   "database",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	link, err := api.LinkComponents(t.Context(), &mdClient, api.LinkComponentsInput{
		FromComponentId: "ecomm-db",
		FromField:       "authentication",
		FromVersion:     "~1.0",
		ToComponentId:   "ecomm-app",
		ToField:         "database",
		ToVersion:       "~2.0",
	})
	if err != nil {
		t.Fatal(err)
	}

	if link.ID != "link-new" {
		t.Errorf("got %s, wanted link-new", link.ID)
	}
}

func TestUnlinkComponents(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"unlinkComponents": map[string]any{
				"result": map[string]any{
					"id":        "link-1",
					"fromField": "authentication",
					"toField":   "database",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	link, err := api.UnlinkComponents(t.Context(), &mdClient, "link-1")
	if err != nil {
		t.Fatal(err)
	}
	if link.ID != "link-1" {
		t.Errorf("got %s, wanted link-1", link.ID)
	}
}
