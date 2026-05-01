package api_test

import (
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetEnvironment(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"environment": map[string]any{
				"id":          "env-uuid1",
				"name":        "Production",
				"description": "Production environment",
				"project": map[string]any{
					"id":   "proj-1",
					"name": "My Project",
				},
			},
		},
	})
	mdClient := client.Client{
		GQLv1: gqlClient,
	}

	env, err := api.GetEnvironment(t.Context(), &mdClient, "env-uuid1")
	if err != nil {
		t.Fatal(err)
	}

	if env.ID != "env-uuid1" {
		t.Errorf("got %s, wanted env-uuid1", env.ID)
	}
	if env.Name != "Production" {
		t.Errorf("got %s, wanted Production", env.Name)
	}
	if env.Project == nil || env.Project.ID != "proj-1" {
		t.Errorf("expected project with ID proj-1")
	}
}

func TestCreateEnvironment(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createEnvironment": map[string]any{
				"result": map[string]any{
					"id":          "env-new",
					"name":        "Staging",
					"description": "Staging environment",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{
		GQLv1: gqlClient,
	}

	env, err := api.CreateEnvironment(t.Context(), &mdClient, "proj-1", api.CreateEnvironmentInput{
		Id:          "staging",
		Name:        "Staging",
		Description: "Staging environment",
	})
	if err != nil {
		t.Fatal(err)
	}

	if env.ID != "env-new" {
		t.Errorf("got %s, wanted env-new", env.ID)
	}
}

func TestDeleteEnvironment(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteEnvironment": map[string]any{
				"result": map[string]any{
					"id":   "env-1",
					"name": "Staging",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{
		GQLv1: gqlClient,
	}

	env, err := api.DeleteEnvironment(t.Context(), &mdClient, "env-1")
	if err != nil {
		t.Fatal(err)
	}

	if env.ID != "env-1" {
		t.Errorf("got %s, wanted env-1", env.ID)
	}
}
