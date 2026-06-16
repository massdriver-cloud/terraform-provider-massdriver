package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/resources"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakeResources struct {
	getResp, createResp, updateResp, deleteResp *resources.Resource
	getErr, createErr, updateErr, deleteErr     error

	getID              string
	createTypeID       string
	createInput        resources.CreateInput
	updateID           string
	updateInput        resources.UpdateInput
	deleteID           string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeResources) Get(_ context.Context, id string) (*resources.Resource, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeResources) Create(_ context.Context, resourceTypeID string, input resources.CreateInput) (*resources.Resource, error) {
	f.createTypeID = resourceTypeID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeResources) Update(_ context.Context, id string, input resources.UpdateInput) (*resources.Resource, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeResources) Delete(_ context.Context, id string) (*resources.Resource, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceImportedResourceCreate(t *testing.T) {
	resp := &resources.Resource{
		ID:           "res-1",
		Name:         "Prod IAM",
		ResourceType: &types.ResourceType{ID: "aws-iam-role"},
		Payload:      map[string]any{"arn": "arn:aws:iam::111:role/prod"},
	}
	fake := &fakeResources{createResp: resp, getResp: resp}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{
		"name":          "Prod IAM",
		"resource_type": "aws-iam-role",
		"resource":      `{"arn":"arn:aws:iam::111:role/prod"}`,
	})

	if diags := resourceImportedResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "res-1" {
		t.Errorf("got id %q, want res-1", rd.Id())
	}
	if fake.createTypeID != "aws-iam-role" {
		t.Errorf("got resourceTypeID %q, want aws-iam-role", fake.createTypeID)
	}
	if fake.createInput.Name != "Prod IAM" {
		t.Errorf("got input.Name %q", fake.createInput.Name)
	}
	if fake.createInput.Payload["arn"] != "arn:aws:iam::111:role/prod" {
		t.Errorf("got payload.arn %v", fake.createInput.Payload["arn"])
	}
}

func TestResourceImportedResourceCreateRejectsInvalidPayload(t *testing.T) {
	fake := &fakeResources{}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{
		"name":          "x",
		"resource_type": "aws-iam-role",
		"resource":      `not json`,
	})

	diags := resourceImportedResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected JSON parse error, got none")
	}
	if fake.createCalls != 0 {
		t.Errorf("Create should not fire when resource JSON is invalid; got %d calls", fake.createCalls)
	}
}

// An empty `resource` HCL field translates to a nil payload — the SDK leaves
// it off the wire entirely. Some resource types accept resources with no
// payload at create time.
func TestResourceImportedResourceCreateAcceptsEmptyResource(t *testing.T) {
	resp := &resources.Resource{
		ID:           "res-2",
		Name:         "No Payload",
		ResourceType: &types.ResourceType{ID: "secret"},
	}
	fake := &fakeResources{createResp: resp, getResp: resp}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{
		"name":          "No Payload",
		"resource_type": "secret",
		// resource intentionally omitted (Default: "")
	})

	if diags := resourceImportedResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.createInput.Payload != nil {
		t.Errorf("Payload should be nil when resource is empty; got %v", fake.createInput.Payload)
	}
}

func TestResourceImportedResourceRead(t *testing.T) {
	pc := &ProviderClient{Resources: &fakeResources{
		getResp: &resources.Resource{
			ID:           "res-1",
			Name:         "Prod IAM",
			ResourceType: &types.ResourceType{ID: "aws-iam-role"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{})
	rd.SetId("res-1")

	if diags := resourceImportedResourceRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "Prod IAM" {
		t.Errorf("got name %q", rd.Get("name"))
	}
	if rd.Get("resource_type").(string) != "aws-iam-role" {
		t.Errorf("got resource_type %q", rd.Get("resource_type"))
	}
}

func TestResourceImportedResourceReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Resources: &fakeResources{
		getErr: fmt.Errorf("get resource: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceImportedResourceRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceImportedResourceUpdate(t *testing.T) {
	resp := &resources.Resource{
		ID:           "res-1",
		Name:         "Renamed",
		ResourceType: &types.ResourceType{ID: "aws-iam-role"},
		Payload:      map[string]any{"arn": "arn:aws:iam::111:role/new"},
	}
	fake := &fakeResources{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{
		"name":          "Renamed",
		"resource_type": "aws-iam-role",
		"resource":      `{"arn":"arn:aws:iam::111:role/new"}`,
	})
	rd.SetId("res-1")

	if diags := resourceImportedResourceUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.updateID != "res-1" {
		t.Errorf("got updateID %q, want res-1", fake.updateID)
	}
	if fake.updateInput.Name != "Renamed" {
		t.Errorf("got input.Name %q", fake.updateInput.Name)
	}
	if fake.updateInput.Payload["arn"] != "arn:aws:iam::111:role/new" {
		t.Errorf("got input.Payload.arn %v", fake.updateInput.Payload["arn"])
	}
}

func TestResourceImportedResourceDelete(t *testing.T) {
	fake := &fakeResources{deleteResp: &resources.Resource{ID: "res-1"}}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{})
	rd.SetId("res-1")

	if diags := resourceImportedResourceDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "res-1" {
		t.Errorf("got deleteID %q, want res-1", fake.deleteID)
	}
}

func TestResourceImportedResourceDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeResources{deleteErr: fmt.Errorf("delete resource: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceImportedResourceDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceImportedResourcePropagatesDeleteFailure(t *testing.T) {
	fake := &fakeResources{deleteErr: fmt.Errorf("delete resource: still referenced by 2 connections")}
	pc := &ProviderClient{Resources: fake}

	rd := schema.TestResourceDataRaw(t, resourceImportedResource().Schema, map[string]any{})
	rd.SetId("res-1")

	diags := resourceImportedResourceDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from blocked delete")
	}
	if !strings.Contains(diags[0].Summary, "still referenced") {
		t.Errorf("upstream error %q should be surfaced verbatim", diags[0].Summary)
	}
}

func TestResourceImportedResourceSchema(t *testing.T) {
	r := resourceImportedResource()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if name := r.Schema["name"]; name == nil || !name.Required {
		t.Error("name should be Required")
	}
	if rt := r.Schema["resource_type"]; rt == nil || !rt.Required || !rt.ForceNew {
		t.Error("resource_type should be Required+ForceNew")
	}
	if res := r.Schema["resource"]; res == nil || !res.Optional || !res.Sensitive {
		t.Error("resource should be Optional+Sensitive")
	}
}
