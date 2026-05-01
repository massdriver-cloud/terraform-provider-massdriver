package api

import (
	"context"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Resource is an infrastructure artifact such as cloud credentials, a database connection string,
// or any other output produced by (or imported into) Massdriver.
// Replaces the v0 concept of "artifact".
type Resource struct {
	ID           string         `json:"id" mapstructure:"id"`
	Name         string         `json:"name" mapstructure:"name"`
	Origin       string         `json:"origin" mapstructure:"origin"`
	ResourceType *ResourceType  `json:"resourceType,omitempty" mapstructure:"resourceType,omitempty"`
	Field        string         `json:"field,omitempty" mapstructure:"field"`
	Formats      []string       `json:"formats,omitempty" mapstructure:"formats"`
	Payload      map[string]any `json:"payload,omitempty" mapstructure:"payload,omitempty"`
}

// ResourceType is the lightweight identifier embedded in a Resource. The full
// ResourceType definition lives server-side; the terraform provider only needs
// to know which type a resource was created against.
type ResourceType struct {
	ID   string `json:"id" mapstructure:"id"`
	Name string `json:"name" mapstructure:"name"`
}

// GetResource retrieves a resource by ID.
func GetResource(ctx context.Context, mdClient *client.Client, id string) (*Resource, error) {
	response, err := getResource(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %w", id, err)
	}
	return toResource(response.Resource)
}

// CreateResource imports a new resource of the given type.
func CreateResource(ctx context.Context, mdClient *client.Client, resourceTypeID string, input CreateResourceInput) (*Resource, error) {
	response, err := createResource(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, resourceTypeID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateResource.Successful {
		messages := make([]string, 0, len(response.CreateResource.Messages))
		for _, m := range response.CreateResource.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create resource", messages)
	}
	return toResource(response.CreateResource.Result)
}

// UpdateResource updates an existing resource.
func UpdateResource(ctx context.Context, mdClient *client.Client, id string, input UpdateResourceInput) (*Resource, error) {
	response, err := updateResource(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateResource.Successful {
		messages := make([]string, 0, len(response.UpdateResource.Messages))
		for _, m := range response.UpdateResource.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update resource", messages)
	}
	return toResource(response.UpdateResource.Result)
}

// DeleteResource deletes an imported resource by ID.
func DeleteResource(ctx context.Context, mdClient *client.Client, id string) (*Resource, error) {
	response, err := deleteResource(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteResource.Successful {
		messages := make([]string, 0, len(response.DeleteResource.Messages))
		for _, m := range response.DeleteResource.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete resource", messages)
	}
	return toResource(response.DeleteResource.Result)
}

func toResource(v any) (*Resource, error) {
	r := Resource{}
	if err := decode(v, &r); err != nil {
		return nil, fmt.Errorf("failed to decode resource: %w", err)
	}
	return &r, nil
}
