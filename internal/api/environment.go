package api

import (
	"context"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Environment represents a Massdriver deployment environment within a project.
type Environment struct {
	ID          string         `json:"id" mapstructure:"id"`
	Name        string         `json:"name" mapstructure:"name"`
	Description string         `json:"description,omitempty" mapstructure:"description"`
	Attributes  map[string]any `json:"attributes,omitempty" mapstructure:"attributes,omitempty"`
	Project     *Project       `json:"project,omitempty" mapstructure:"project,omitempty"`
}

// GetEnvironment retrieves an environment by ID from the Massdriver API.
func GetEnvironment(ctx context.Context, mdClient *client.Client, id string) (*Environment, error) {
	response, err := getEnvironment(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment %s: %w", id, err)
	}

	return toEnvironment(response.Environment)
}

func toEnvironment(v any) (*Environment, error) {
	env := Environment{}
	if err := decode(v, &env); err != nil {
		return nil, fmt.Errorf("failed to decode environment: %w", err)
	}
	return &env, nil
}

// CreateEnvironment creates a new environment within the given project.
func CreateEnvironment(ctx context.Context, mdClient *client.Client, projectID string, input CreateEnvironmentInput) (*Environment, error) {
	response, err := createEnvironment(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, projectID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateEnvironment.Successful {
		messages := make([]string, 0, len(response.CreateEnvironment.Messages))
		for _, m := range response.CreateEnvironment.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create environment", messages)
	}
	return toEnvironment(response.CreateEnvironment.Result)
}

// UpdateEnvironment updates an environment in the Massdriver API.
func UpdateEnvironment(ctx context.Context, mdClient *client.Client, id string, input UpdateEnvironmentInput) (*Environment, error) {
	response, err := updateEnvironment(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateEnvironment.Successful {
		messages := make([]string, 0, len(response.UpdateEnvironment.Messages))
		for _, m := range response.UpdateEnvironment.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update environment", messages)
	}
	return toEnvironment(response.UpdateEnvironment.Result)
}

// DeleteEnvironment removes an environment by ID from the Massdriver API.
func DeleteEnvironment(ctx context.Context, mdClient *client.Client, id string) (*Environment, error) {
	response, err := deleteEnvironment(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteEnvironment.Successful {
		messages := make([]string, 0, len(response.DeleteEnvironment.Messages))
		for _, m := range response.DeleteEnvironment.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete environment", messages)
	}
	return toEnvironment(response.DeleteEnvironment.Result)
}
