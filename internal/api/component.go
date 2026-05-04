package api

import (
	"context"
	"fmt"
	"time"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Component represents a slot in a project's blueprint backed by a bundle.
type Component struct {
	ID          string            `json:"id" mapstructure:"id"`
	Name        string            `json:"name" mapstructure:"name"`
	Description string            `json:"description,omitempty" mapstructure:"description"`
	Attributes  map[string]any    `json:"attributes,omitempty" mapstructure:"attributes,omitempty"`
	OciRepo     *OciRepo          `json:"ociRepo,omitempty" mapstructure:"ociRepo,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitzero" mapstructure:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt,omitzero" mapstructure:"updatedAt"`
}

// Link represents a design-time wire between two components in a blueprint.
type Link struct {
	ID            string     `json:"id" mapstructure:"id"`
	FromField     string     `json:"fromField" mapstructure:"fromField"`
	ToField       string     `json:"toField" mapstructure:"toField"`
	FromComponent *Component `json:"fromComponent,omitempty" mapstructure:"fromComponent,omitempty"`
	ToComponent   *Component `json:"toComponent,omitempty" mapstructure:"toComponent,omitempty"`
	CreatedAt     time.Time  `json:"createdAt,omitzero" mapstructure:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt,omitzero" mapstructure:"updatedAt"`
}

// ListComponents returns components in a project's blueprint, optionally filtered.
func ListComponents(ctx context.Context, mdClient *client.Client, projectID string, filter *ComponentsFilter) ([]Component, error) {
	response, err := listComponents(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, projectID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list components for project %s: %w", projectID, err)
	}

	items := response.Project.Blueprint.Components.Items
	components := make([]Component, 0, len(items))
	for _, item := range items {
		c, decodeErr := toComponent(item)
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to convert component: %w", decodeErr)
		}
		components = append(components, *c)
	}
	return components, nil
}

// AddComponent adds a component to a project's blueprint.
func AddComponent(ctx context.Context, mdClient *client.Client, projectID, ociRepoName string, input AddComponentInput) (*Component, error) {
	response, err := addComponent(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, projectID, ociRepoName, input)
	if err != nil {
		return nil, err
	}
	if !response.AddComponent.Successful {
		messages := make([]string, 0, len(response.AddComponent.Messages))
		for _, m := range response.AddComponent.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to add component", messages)
	}
	return toComponent(response.AddComponent.Result)
}

// UpdateComponent updates a component's name, description, and/or attributes.
// The component ID and underlying bundle are immutable.
func UpdateComponent(ctx context.Context, mdClient *client.Client, componentID string, input UpdateComponentInput) (*Component, error) {
	response, err := updateComponent(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, componentID, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateComponent.Successful {
		messages := make([]string, 0, len(response.UpdateComponent.Messages))
		for _, m := range response.UpdateComponent.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update component", messages)
	}
	return toComponent(response.UpdateComponent.Result)
}

// RemoveComponent removes a component from a project's blueprint.
func RemoveComponent(ctx context.Context, mdClient *client.Client, componentID string) (*Component, error) {
	response, err := removeComponent(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, componentID)
	if err != nil {
		return nil, err
	}
	if !response.RemoveComponent.Successful {
		messages := make([]string, 0, len(response.RemoveComponent.Messages))
		for _, m := range response.RemoveComponent.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to remove component", messages)
	}
	return toComponent(response.RemoveComponent.Result)
}

// ListLinks returns links in a project's blueprint, optionally filtered by source/destination component.
func ListLinks(ctx context.Context, mdClient *client.Client, projectID string, filter *LinksFilter) ([]Link, error) {
	response, err := listLinks(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, projectID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list links for project %s: %w", projectID, err)
	}

	items := response.Project.Blueprint.Links.Items
	links := make([]Link, 0, len(items))
	for _, item := range items {
		l, decodeErr := toLink(item)
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to convert link: %w", decodeErr)
		}
		links = append(links, *l)
	}
	return links, nil
}

// LinkComponents creates a design-time link between two components.
func LinkComponents(ctx context.Context, mdClient *client.Client, input LinkComponentsInput) (*Link, error) {
	response, err := linkComponents(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	if !response.LinkComponents.Successful {
		messages := make([]string, 0, len(response.LinkComponents.Messages))
		for _, m := range response.LinkComponents.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to link components", messages)
	}
	return toLink(response.LinkComponents.Result)
}

// UnlinkComponents removes a link by its ID.
func UnlinkComponents(ctx context.Context, mdClient *client.Client, linkID string) (*Link, error) {
	response, err := unlinkComponents(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, linkID)
	if err != nil {
		return nil, err
	}
	if !response.UnlinkComponents.Successful {
		messages := make([]string, 0, len(response.UnlinkComponents.Messages))
		for _, m := range response.UnlinkComponents.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to unlink components", messages)
	}
	return toLink(response.UnlinkComponents.Result)
}

func toComponent(v any) (*Component, error) {
	c := Component{}
	if err := decode(v, &c); err != nil {
		return nil, fmt.Errorf("failed to decode component: %w", err)
	}
	return &c, nil
}

func toLink(v any) (*Link, error) {
	l := Link{}
	if err := decode(v, &l); err != nil {
		return nil, fmt.Errorf("failed to decode link: %w", err)
	}
	return &l, nil
}
