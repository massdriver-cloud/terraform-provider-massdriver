package api

import (
	"context"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Group is a collection of users and service accounts that share an access level
// within an organization. Groups are the primary access-control primitive in Massdriver.
//
// `Role` is assigned by the API (built-in groups carry `organization_admin`/`organization_viewer`;
// new custom groups always get `CUSTOM`). It can't be changed after creation.
type Group struct {
	ID          string `json:"id" mapstructure:"id"`
	Name        string `json:"name" mapstructure:"name"`
	Description string `json:"description,omitempty" mapstructure:"description"`
	Role        string `json:"role" mapstructure:"role"`
}

// GetGroup retrieves a group by ID.
func GetGroup(ctx context.Context, mdClient *client.Client, id string) (*Group, error) {
	response, err := getGroup(ctx, mdClient.GQLv2, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get group %s: %w", id, err)
	}
	return toGroup(response.Group)
}

// CreateGroup creates a new custom group.
func CreateGroup(ctx context.Context, mdClient *client.Client, input CreateGroupInput) (*Group, error) {
	response, err := createGroup(ctx, mdClient.GQLv2, mdClient.Config.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateGroup.Successful {
		messages := make([]string, 0, len(response.CreateGroup.Messages))
		for _, m := range response.CreateGroup.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create group", messages)
	}
	return toGroup(response.CreateGroup.Result)
}

// UpdateGroup updates a group's name or description. The role is immutable.
func UpdateGroup(ctx context.Context, mdClient *client.Client, id string, input UpdateGroupInput) (*Group, error) {
	response, err := updateGroup(ctx, mdClient.GQLv2, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateGroup.Successful {
		messages := make([]string, 0, len(response.UpdateGroup.Messages))
		for _, m := range response.UpdateGroup.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update group", messages)
	}
	return toGroup(response.UpdateGroup.Result)
}

// DeleteGroup deletes a custom group. Built-in groups (`organization_admin`,
// `organization_viewer`) cannot be deleted; the API rejects those requests.
func DeleteGroup(ctx context.Context, mdClient *client.Client, id string) (*Group, error) {
	response, err := deleteGroup(ctx, mdClient.GQLv2, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteGroup.Successful {
		messages := make([]string, 0, len(response.DeleteGroup.Messages))
		for _, m := range response.DeleteGroup.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete group", messages)
	}
	return toGroup(response.DeleteGroup.Result)
}

func toGroup(v any) (*Group, error) {
	g := Group{}
	if err := decode(v, &g); err != nil {
		return nil, fmt.Errorf("failed to decode group: %w", err)
	}
	return &g, nil
}
