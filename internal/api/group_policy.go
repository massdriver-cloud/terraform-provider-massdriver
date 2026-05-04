package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// Policy is a single ABAC group policy: an effect (`ALLOW`/`DENY`), one or
// more actions, and either the literal wildcard `"*"` or a JSON-encoded
// conditions string. Conditions inside one policy AND together; policies on
// the same group OR together; `DENY` always wins.
type Policy struct {
	ID         string
	Effect     string
	Actions    []string
	Conditions string
	GroupID    string
}

// EncodeConditions encodes the user-facing conditions string as a JSON
// string for transport.
//
// The Conditions scalar is asymmetric: input is always a JSON-encoded string
// (the server unmarshals one level to recover the user's string, then re-
// parses it as either "*" or a JSON conditions object), but output comes
// back as a structured JSON value (string for the wildcard, object for
// conditions). So we json.Marshal on the way in and round-trip through
// conditionsFromWire on the way out.
//
// Exported because the terraform resource layer builds CreateGroupPolicyInput
// / UpdatePolicyInput directly and needs to wrap the user's string before
// passing it through.
func EncodeConditions(s string) json.RawMessage {
	encoded, err := json.Marshal(s)
	if err != nil {
		// Marshaling a Go string never errors; this is unreachable in practice.
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

// decodeConditions flattens the structured response value back to a string.
// A JSON string `"*"` collapses to the bare `*`; a JSON object is surfaced as
// its raw textual form (e.g., `{"team":["eng"]}`).
func decodeConditions(raw json.RawMessage) string {
	if string(raw) == `"*"` {
		return "*"
	}
	return string(raw)
}

// GetGroupPolicy retrieves a single policy by ID, scoped to its group.
//
// The schema doesn't expose a top-level `policy(id)` query — we list the
// group's policies and filter client-side. The group has at most a few
// policies in practice, so paginated fetch isn't justified yet; if a group
// ever grows past the default page size this will need revisiting.
//
// Returns (nil, nil) when the policy doesn't exist — terraform Reads use
// that signal to clear state.
func GetGroupPolicy(ctx context.Context, mdClient *client.Client, groupID, policyID string) (*Policy, error) {
	response, err := listGroupPolicies(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to list policies for group %s: %w", groupID, err)
	}

	for _, item := range response.Group.Policies.Items {
		if item.Id == policyID {
			return &Policy{
				ID:         item.Id,
				Effect:     string(item.Effect),
				Actions:    item.Actions,
				Conditions: decodeConditions(item.Conditions),
				GroupID:    item.Group.Id,
			}, nil
		}
	}
	return nil, nil
}

// CreateGroupPolicy attaches a new policy to a group.
func CreateGroupPolicy(ctx context.Context, mdClient *client.Client, groupID string, input CreateGroupPolicyInput) (*Policy, error) {
	response, err := createGroupPolicy(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, groupID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateGroupPolicy.Successful {
		messages := make([]string, 0, len(response.CreateGroupPolicy.Messages))
		for _, m := range response.CreateGroupPolicy.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create group policy", messages)
	}
	r := response.CreateGroupPolicy.Result
	return &Policy{
		ID:         r.Id,
		Effect:     string(r.Effect),
		Actions:    r.Actions,
		Conditions: decodeConditions(r.Conditions),
		GroupID:    r.Group.Id,
	}, nil
}

// UpdatePolicy edits a policy's effect/actions/conditions in place. The
// principal (group) cannot be changed.
func UpdatePolicy(ctx context.Context, mdClient *client.Client, id string, input UpdatePolicyInput) (*Policy, error) {
	response, err := updatePolicy(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdatePolicy.Successful {
		messages := make([]string, 0, len(response.UpdatePolicy.Messages))
		for _, m := range response.UpdatePolicy.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update policy", messages)
	}
	r := response.UpdatePolicy.Result
	return &Policy{
		ID:         r.Id,
		Effect:     string(r.Effect),
		Actions:    r.Actions,
		Conditions: decodeConditions(r.Conditions),
		GroupID:    r.Group.Id,
	}, nil
}

// DeletePolicy removes a policy by ID.
func DeletePolicy(ctx context.Context, mdClient *client.Client, id string) (*Policy, error) {
	response, err := deletePolicy(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeletePolicy.Successful {
		messages := make([]string, 0, len(response.DeletePolicy.Messages))
		for _, m := range response.DeletePolicy.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete policy", messages)
	}
	return &Policy{ID: response.DeletePolicy.Result.Id}, nil
}
