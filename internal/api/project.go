// Package api provides a client for the Massdriver v1 GraphQL API.
package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Project represents a Massdriver project.
type Project struct {
	ID          string `json:"id" mapstructure:"id"`
	Name        string `json:"name" mapstructure:"name"`
	Description string `json:"description" mapstructure:"description"`
}

// GetProject retrieves a project by ID from the Massdriver API.
func GetProject(ctx context.Context, mdClient *client.Client, id string) (*Project, error) {
	response, err := getProject(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get project %s: %w", id, err)
	}

	return toProject(response.Project)
}

func toProject(p any) (*Project, error) {
	proj := Project{}
	if err := decode(p, &proj); err != nil {
		return nil, fmt.Errorf("failed to decode project: %w", err)
	}
	return &proj, nil
}

// CreateProject creates a new project in the Massdriver API.
func CreateProject(ctx context.Context, mdClient *client.Client, input CreateProjectInput) (*Project, error) {
	response, err := createProject(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateProject.Successful {
		messages := make([]string, 0, len(response.CreateProject.Messages))
		for _, m := range response.CreateProject.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create project", messages)
	}
	return toProject(response.CreateProject.Result)
}

// UpdateProject updates a project in the Massdriver API.
func UpdateProject(ctx context.Context, mdClient *client.Client, id string, input UpdateProjectInput) (*Project, error) {
	response, err := updateProject(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateProject.Successful {
		messages := make([]string, 0, len(response.UpdateProject.Messages))
		for _, m := range response.UpdateProject.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update project", messages)
	}
	return toProject(response.UpdateProject.Result)
}

// DeleteProject removes a project by ID from the Massdriver API.
func DeleteProject(ctx context.Context, mdClient *client.Client, id string) (*Project, error) {
	response, err := deleteProject(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteProject.Successful {
		messages := make([]string, 0, len(response.DeleteProject.Messages))
		for _, m := range response.DeleteProject.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete project", messages)
	}
	return toProject(response.DeleteProject.Result)
}

// mutationFailure formats a prefix and a list of validation messages into a single multi-line error.
func mutationFailure(prefix string, messages []string) error {
	if len(messages) == 0 {
		return errors.New(prefix)
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(":")
	for _, msg := range messages {
		sb.WriteString("\n  - ")
		sb.WriteString(msg)
	}
	return errors.New(sb.String())
}
