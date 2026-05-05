package api_test

import (
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetOciRepo(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"ociRepo": map[string]any{
				"id":           "aws-aurora-postgres",
				"name":         "aws-aurora-postgres",
				"reference":    "api.massdriver.cloud/acme/aws-aurora-postgres",
				"artifactType": "application/vnd.massdriver.bundle.v1+json",
				"attributes":   map[string]any{"team": "platform"},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	r, err := api.GetOciRepo(t.Context(), &mdClient, "aws-aurora-postgres")
	if err != nil {
		t.Fatal(err)
	}

	if r.ID != "aws-aurora-postgres" {
		t.Errorf("got ID %s, wanted aws-aurora-postgres", r.ID)
	}
	if r.Reference != "api.massdriver.cloud/acme/aws-aurora-postgres" {
		t.Errorf("got reference %q", r.Reference)
	}
	if r.ArtifactType != "application/vnd.massdriver.bundle.v1+json" {
		t.Errorf("got artifactType %q", r.ArtifactType)
	}
	if r.Attributes["team"] != "platform" {
		t.Errorf("got attributes %v", r.Attributes)
	}
}

func TestCreateOciRepo(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createOciRepo": map[string]any{
				"result": map[string]any{
					"id":           "aws-aurora-postgres",
					"name":         "aws-aurora-postgres",
					"reference":    "api.massdriver.cloud/acme/aws-aurora-postgres",
					"artifactType": "application/vnd.massdriver.bundle.v1+json",
					"attributes":   map[string]any{},
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	r, err := api.CreateOciRepo(t.Context(), &mdClient, api.CreateOciRepoInput{
		Id:           "aws-aurora-postgres",
		ArtifactType: api.OciArtifactTypeBundle,
		Attributes:   map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "aws-aurora-postgres" {
		t.Errorf("got ID %s, wanted aws-aurora-postgres", r.ID)
	}
}

// `md-*` keys are reserved — server rejects them. The wrapper has to surface
// that as a clean error rather than swallow it.
func TestCreateOciRepoFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createOciRepo": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "attributes", "message": "key 'md-id' is reserved"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.CreateOciRepo(t.Context(), &mdClient, api.CreateOciRepoInput{
		Id:           "aws-aurora-postgres",
		ArtifactType: api.OciArtifactTypeBundle,
		Attributes:   map[string]any{"md-id": "nope"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateOciRepo(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"updateOciRepo": map[string]any{
				"result": map[string]any{
					"id":         "aws-aurora-postgres",
					"name":       "aws-aurora-postgres",
					"attributes": map[string]any{"team": "infra"},
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	r, err := api.UpdateOciRepo(t.Context(), &mdClient, "aws-aurora-postgres", api.UpdateOciRepoInput{
		Attributes: map[string]any{"team": "infra"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Attributes["team"] != "infra" {
		t.Errorf("got attributes %v, wanted team=infra", r.Attributes)
	}
}

func TestDeleteOciRepo(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteOciRepo": map[string]any{
				"result":     map[string]any{"id": "aws-aurora-postgres", "name": "aws-aurora-postgres"},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	r, err := api.DeleteOciRepo(t.Context(), &mdClient, "aws-aurora-postgres")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "aws-aurora-postgres" {
		t.Errorf("got ID %s", r.ID)
	}
}

// Delete is refused by the server when the repo has published versions.
// The wrapper must surface that as an error so terraform doesn't swallow it.
func TestDeleteOciRepoRejectsWhenVersionsExist(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteOciRepo": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "conflict", "field": "id", "message": "repository has published versions; recreate to clear"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.DeleteOciRepo(t.Context(), &mdClient, "aws-aurora-postgres")
	if err == nil {
		t.Fatal("expected error rejecting deletion of repo with versions")
	}
}
