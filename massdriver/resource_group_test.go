package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/groups"
)

type fakeGroups struct {
	getResp, createResp, updateResp, deleteResp *groups.Group
	getErr, createErr, updateErr, deleteErr     error

	getID       string
	createInput groups.CreateInput
	updateID    string
	updateInput groups.UpdateInput
	deleteID    string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeGroups) Get(_ context.Context, id string) (*groups.Group, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeGroups) Create(_ context.Context, input groups.CreateInput) (*groups.Group, error) {
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeGroups) Update(_ context.Context, id string, input groups.UpdateInput) (*groups.Group, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeGroups) Delete(_ context.Context, id string) (*groups.Group, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceGroupCreate(t *testing.T) {
	resp := &groups.Group{
		ID:          "group-1",
		Name:        "SREs",
		Description: "on-call engineers",
	}
	fake := &fakeGroups{createResp: resp, getResp: resp}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name":        "SREs",
		"description": "on-call engineers",
	})

	if diags := resourceGroupCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "group-1" {
		t.Errorf("got id %q, want group-1", rd.Id())
	}
	in := fake.createInput
	if in.Name != "SREs" || in.Description != "on-call engineers" {
		t.Errorf("got create input %+v", in)
	}
}

func TestResourceGroupCreateOmitsUnsetDescription(t *testing.T) {
	resp := &groups.Group{ID: "group-1", Name: "Minimal"}
	fake := &fakeGroups{createResp: resp, getResp: resp}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name": "Minimal",
	})

	if diags := resourceGroupCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.createInput.Description != "" {
		t.Errorf("got Description %q, want empty", fake.createInput.Description)
	}
}

func TestResourceGroupCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeGroups{createErr: fmt.Errorf("create group: name must be unique")}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name": "SREs",
	})

	diags := resourceGroupCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "name must be unique") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceGroupRead(t *testing.T) {
	pc := &ProviderClient{Groups: &fakeGroups{
		getResp: &groups.Group{
			ID:          "group-1",
			Name:        "SREs",
			Description: "from the server",
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("group-1")

	if diags := resourceGroupRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("name").(string) != "SREs" {
		t.Errorf("got name %q", rd.Get("name"))
	}
	if rd.Get("description").(string) != "from the server" {
		t.Errorf("got description %q", rd.Get("description"))
	}
}

func TestResourceGroupReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Groups: &fakeGroups{
		getErr: fmt.Errorf("get group: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceGroupRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceGroupUpdate(t *testing.T) {
	resp := &groups.Group{
		ID:          "group-1",
		Name:        "Renamed",
		Description: "updated",
	}
	fake := &fakeGroups{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("group-1")

	if diags := resourceGroupUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "group-1" {
		t.Errorf("got updateID %q, want group-1", fake.updateID)
	}
	in := fake.updateInput
	if in.Name != "Renamed" || in.Description != "updated" {
		t.Errorf("got input %+v", in)
	}
}

func TestResourceGroupDelete(t *testing.T) {
	fake := &fakeGroups{deleteResp: &groups.Group{ID: "group-1"}}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("group-1")

	if diags := resourceGroupDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "group-1" {
		t.Errorf("got deleteID %q, want group-1", fake.deleteID)
	}
}

// Deleting a built-in group surfaces a 403/forbidden — that error should
// propagate verbatim, not be silenced as "already gone."
func TestResourceGroupDeletePropagatesBuiltInRejection(t *testing.T) {
	fake := &fakeGroups{deleteErr: fmt.Errorf("delete group: cannot delete built-in group: %w", gql.ErrForbidden)}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("organization_admin")

	diags := resourceGroupDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from built-in group deletion, got none")
	}
	if rd.Id() == "" {
		t.Error("ID should remain set when delete fails")
	}
	if !strings.Contains(diags[0].Summary, "built-in") {
		t.Errorf("upstream error should be surfaced verbatim; got %q", diags[0].Summary)
	}
}

func TestResourceGroupDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeGroups{deleteErr: fmt.Errorf("delete group: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Groups: fake}

	rd := schema.TestResourceDataRaw(t, resourceGroup().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceGroupDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceGroupIgnoresDescriptionDriftWhenConfigUnset(t *testing.T) {
	r := resourceGroup()

	state := &terraform.InstanceState{
		ID: "group-1",
		Attributes: map[string]string{
			"id":          "group-1",
			"name":        "SREs",
			"description": "added in the console",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"name": "SREs",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff != nil && !diff.Empty() {
		if attr := diff.Attributes["description"]; attr != nil {
			t.Errorf("expected no diff on description when config omits it; got %+v", attr)
		}
	}
}

func TestResourceGroupSchema(t *testing.T) {
	r := resourceGroup()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if name := r.Schema["name"]; name == nil || !name.Required {
		t.Error("name should be Required")
	}
	if desc := r.Schema["description"]; desc == nil || !desc.Optional || !desc.Computed {
		t.Error("description should be Optional+Computed")
	}
}
