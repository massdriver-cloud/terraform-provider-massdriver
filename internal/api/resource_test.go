package api_test

import (
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetResource(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"resource": map[string]any{
				"id":     "res-uuid1",
				"name":   "my-vpc",
				"origin": "PROVISIONED",
				"resourceType": map[string]any{
					"id":   "aws-vpc",
					"name": "AWS VPC",
				},
				"field":   "network",
				"formats": []string{"json", "yaml"},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	r, err := api.GetResource(t.Context(), &mdClient, "res-uuid1")
	if err != nil {
		t.Fatal(err)
	}

	if r.ID != "res-uuid1" {
		t.Errorf("got ID %s, wanted res-uuid1", r.ID)
	}
	if r.Name != "my-vpc" {
		t.Errorf("got name %s, wanted my-vpc", r.Name)
	}
	if r.Origin != "PROVISIONED" {
		t.Errorf("got origin %s, wanted PROVISIONED", r.Origin)
	}
	if r.ResourceType == nil || r.ResourceType.ID != "aws-vpc" {
		t.Errorf("expected resource type aws-vpc")
	}
	if r.Field != "network" {
		t.Errorf("got field %s, wanted network", r.Field)
	}
	if len(r.Formats) != 2 || r.Formats[0] != "json" || r.Formats[1] != "yaml" {
		t.Errorf("got formats %v, wanted [json yaml]", r.Formats)
	}
}

func TestCreateResource(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createResource": map[string]any{
				"result": map[string]any{
					"id":     "res-new",
					"name":   "CI/CD Role",
					"origin": "IMPORTED",
					"resourceType": map[string]any{
						"id":   "aws-iam-role",
						"name": "AWS IAM Role",
					},
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	r, err := api.CreateResource(t.Context(), &mdClient, "aws-iam-role", api.CreateResourceInput{
		Name: "CI/CD Role",
	})
	if err != nil {
		t.Fatal(err)
	}

	if r.ID != "res-new" {
		t.Errorf("got ID %s, wanted res-new", r.ID)
	}
	if r.Name != "CI/CD Role" {
		t.Errorf("got name %s, wanted CI/CD Role", r.Name)
	}
}

func TestCreateResourceFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createResource": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{
						"code":    "validation",
						"field":   "name",
						"message": "name is required",
					},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.CreateResource(t.Context(), &mdClient, "aws-iam-role", api.CreateResourceInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateResource(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"updateResource": map[string]any{
				"result": map[string]any{
					"id":     "res-1",
					"name":   "Updated Name",
					"origin": "IMPORTED",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	r, err := api.UpdateResource(t.Context(), &mdClient, "res-1", api.UpdateResourceInput{
		Name: "Updated Name",
	})
	if err != nil {
		t.Fatal(err)
	}

	if r.Name != "Updated Name" {
		t.Errorf("got name %s, wanted Updated Name", r.Name)
	}
}

func TestDeleteResource(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteResource": map[string]any{
				"result": map[string]any{
					"id":     "res-1",
					"name":   "my-vpc",
					"origin": "IMPORTED",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	r, err := api.DeleteResource(t.Context(), &mdClient, "res-1")
	if err != nil {
		t.Fatal(err)
	}

	if r.ID != "res-1" {
		t.Errorf("got ID %s, wanted res-1", r.ID)
	}
}

func TestDeleteResourceFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteResource": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{
						"code":    "conflict",
						"field":   "id",
						"message": "resource is referenced by active connections",
					},
				},
			},
		},
	})
	mdClient := client.Client{GQLv1: gqlClient}

	_, err := api.DeleteResource(t.Context(), &mdClient, "res-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
