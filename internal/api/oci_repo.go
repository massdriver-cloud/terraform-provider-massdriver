package api

import (
	"context"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// OciRepo is an OCI repository in an organization's bundle catalog. The
// repository must exist before any version can be published — pushes against a
// non-existent repository return 404.
type OciRepo struct {
	ID           string         `json:"id" mapstructure:"id"`
	Name         string         `json:"name" mapstructure:"name"`
	Reference    string         `json:"reference" mapstructure:"reference"`
	ArtifactType string         `json:"artifactType" mapstructure:"artifactType"`
	Attributes   map[string]any `json:"attributes,omitempty" mapstructure:"attributes,omitempty"`
}

// GetOciRepo retrieves an OCI repository by name (the API treats the name as the ID).
func GetOciRepo(ctx context.Context, mdClient *client.Client, id string) (*OciRepo, error) {
	response, err := getOciRepo(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCI repo %s: %w", id, err)
	}
	return toOciRepo(response.OciRepo)
}

// CreateOciRepo creates a new (empty) OCI repository in the org's catalog.
// The `id` becomes the repository's permanent name; immutable after creation.
func CreateOciRepo(ctx context.Context, mdClient *client.Client, input CreateOciRepoInput) (*OciRepo, error) {
	response, err := createOciRepo(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateOciRepo.Successful {
		messages := make([]string, 0, len(response.CreateOciRepo.Messages))
		for _, m := range response.CreateOciRepo.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create OCI repo", messages)
	}
	return toOciRepo(response.CreateOciRepo.Result)
}

// UpdateOciRepo replaces a repository's user-settable attributes. The
// repository name and artifact type are immutable.
func UpdateOciRepo(ctx context.Context, mdClient *client.Client, id string, input UpdateOciRepoInput) (*OciRepo, error) {
	response, err := updateOciRepo(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateOciRepo.Successful {
		messages := make([]string, 0, len(response.UpdateOciRepo.Messages))
		for _, m := range response.UpdateOciRepo.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update OCI repo", messages)
	}
	return toOciRepo(response.UpdateOciRepo.Result)
}

// DeleteOciRepo removes an OCI repository. The API refuses if the repository
// has any published versions — surface that as an error.
func DeleteOciRepo(ctx context.Context, mdClient *client.Client, id string) (*OciRepo, error) {
	response, err := deleteOciRepo(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteOciRepo.Successful {
		messages := make([]string, 0, len(response.DeleteOciRepo.Messages))
		for _, m := range response.DeleteOciRepo.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete OCI repo", messages)
	}
	return toOciRepo(response.DeleteOciRepo.Result)
}

func toOciRepo(v any) (*OciRepo, error) {
	r := OciRepo{}
	if err := decode(v, &r); err != nil {
		return nil, fmt.Errorf("failed to decode OCI repo: %w", err)
	}
	return &r, nil
}
