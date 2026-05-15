package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/environments"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakeEnvironments struct {
	getResp, createResp, updateResp, deleteResp *environments.Environment
	getErr, createErr, updateErr, deleteErr     error

	getID           string
	createProjectID string
	createInput     environments.CreateInput
	updateID        string
	updateInput     environments.UpdateInput
	deleteID        string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeEnvironments) Get(_ context.Context, id string) (*environments.Environment, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeEnvironments) Create(_ context.Context, projectID string, input environments.CreateInput) (*environments.Environment, error) {
	f.createProjectID = projectID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeEnvironments) Update(_ context.Context, id string, input environments.UpdateInput) (*environments.Environment, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeEnvironments) Delete(_ context.Context, id string) (*environments.Environment, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceEnvironmentCreate(t *testing.T) {
	resp := &environments.Environment{
		ID:          "ecomm-prod",
		Name:        "Prod",
		Description: "production",
		Attributes:  map[string]any{"env": "prod"},
		Project:     &types.Project{ID: "ecomm"},
	}
	fake := &fakeEnvironments{createResp: resp, getResp: resp}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier":  "prod",
		"project_id":  "ecomm",
		"name":        "Prod",
		"description": "production",
		"attributes":  map[string]any{"env": "prod"},
	})

	if diags := resourceEnvironmentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm-prod" {
		t.Errorf("got id %q, want ecomm-prod", rd.Id())
	}
	if fake.createProjectID != "ecomm" {
		t.Errorf("got Create projectID %q, want ecomm", fake.createProjectID)
	}
	in := fake.createInput
	if in.ID != "prod" || in.Name != "Prod" || in.Description != "production" {
		t.Errorf("got create input %+v", in)
	}
	if in.Attributes["env"] != "prod" {
		t.Errorf("got Attributes %v", in.Attributes)
	}
	if rd.Get("project_id").(string) != "ecomm" {
		t.Errorf("got project_id %q, want ecomm (from Read)", rd.Get("project_id"))
	}
	if rd.Get("identifier").(string) != "prod" {
		t.Errorf("got identifier %q, want prod (stripped from %q)", rd.Get("identifier"), resp.ID)
	}
}

func TestResourceEnvironmentSurfacesAttributesDrift(t *testing.T) {
	r := resourceEnvironment()

	state := &terraform.InstanceState{
		ID: "ecomm-prod",
		Attributes: map[string]string{
			"id":             "ecomm-prod",
			"identifier":     "prod",
			"project_id":     "ecomm",
			"attributes.%":   "1",
			"attributes.env": "staging",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
		"attributes": map[string]any{"env": "prod"},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected attributes drift to surface as a diff")
	}
	if attr := diff.Attributes["attributes.env"]; attr == nil {
		t.Error("expected diff on attributes.env")
	} else if attr.Old != "staging" || attr.New != "prod" {
		t.Errorf("got attributes.env diff %+v, want Old=staging New=prod", attr)
	}
}

func TestResourceEnvironmentRead(t *testing.T) {
	pc := &ProviderClient{Environments: &fakeEnvironments{
		getResp: &environments.Environment{
			ID:          "ecomm-staging",
			Name:        "Staging",
			Description: "pre-prod",
			Attributes:  map[string]any{"env": "staging"},
			Project:     &types.Project{ID: "ecomm"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("ecomm-staging")

	if diags := resourceEnvironmentRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := rd.Get("identifier").(string); got != "staging" {
		t.Errorf("got identifier %q, want staging (stripped from ecomm-staging)", got)
	}
	if got := rd.Get("project_id").(string); got != "ecomm" {
		t.Errorf("got project_id %q, want ecomm", got)
	}
	if got := rd.Get("name").(string); got != "Staging" {
		t.Errorf("got name %q, want Staging", got)
	}
	if got := rd.Get("description").(string); got != "pre-prod" {
		t.Errorf("got description %q, want pre-prod", got)
	}
}

func TestResourceEnvironmentReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Environments: &fakeEnvironments{
		getErr: fmt.Errorf("get environment gone: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceEnvironmentRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceEnvironmentUpdate(t *testing.T) {
	resp := &environments.Environment{
		ID:          "ecomm-prod",
		Name:        "Renamed",
		Description: "updated",
		Project:     &types.Project{ID: "ecomm"},
	}
	fake := &fakeEnvironments{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier":  "prod",
		"project_id":  "ecomm",
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("ecomm-prod")

	if diags := resourceEnvironmentUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "ecomm-prod" {
		t.Errorf("got updateID %q, want ecomm-prod", fake.updateID)
	}
	in := fake.updateInput
	if in.Name != "Renamed" || in.Description != "updated" {
		t.Errorf("got input %+v", in)
	}
}

func TestResourceEnvironmentDelete(t *testing.T) {
	fake := &fakeEnvironments{deleteResp: &environments.Environment{ID: "ecomm-prod"}}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("ecomm-prod")

	if diags := resourceEnvironmentDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "ecomm-prod" {
		t.Errorf("got deleteID %q, want ecomm-prod", fake.deleteID)
	}
}

func TestResourceEnvironmentDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeEnvironments{deleteErr: fmt.Errorf("delete environment: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceEnvironmentDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceEnvironmentCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeEnvironments{createErr: fmt.Errorf("create environment: id already exists in project")}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
		"name":       "Prod",
	})

	diags := resourceEnvironmentCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "id already exists") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceEnvironmentCreateDefaultsNameToIdentifier(t *testing.T) {
	resp := &environments.Environment{ID: "ecomm-prod", Name: "prod", Project: &types.Project{ID: "ecomm"}}
	fake := &fakeEnvironments{createResp: resp, getResp: resp}
	pc := &ProviderClient{Environments: fake}

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
		// name and description deliberately omitted
	})

	if diags := resourceEnvironmentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.createInput.Name != "prod" {
		t.Errorf("got input.Name %q, want prod (defaulted from identifier)", fake.createInput.Name)
	}
	if fake.createInput.Description != "" {
		t.Errorf("got input.Description %q, want empty", fake.createInput.Description)
	}
}

func TestResourceEnvironmentIgnoresDriftWhenConfigUnset(t *testing.T) {
	r := resourceEnvironment()

	state := &terraform.InstanceState{
		ID: "ecomm-prod",
		Attributes: map[string]string{
			"id":          "ecomm-prod",
			"identifier":  "prod",
			"project_id":  "ecomm",
			"name":        "Console Edit",
			"description": "added in the console",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}

	if diff != nil && !diff.Empty() {
		for k, attr := range diff.Attributes {
			if k == "name" || k == "description" {
				t.Errorf("expected no diff on %s when config omits it; got %+v", k, attr)
			}
		}
	}
}

func TestResourceEnvironmentSchema(t *testing.T) {
	r := resourceEnvironment()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	for _, field := range []string{"identifier", "project_id"} {
		s := r.Schema[field]
		if s == nil {
			t.Fatalf("expected %s in schema", field)
		}
		if !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew; got Required=%v ForceNew=%v", field, s.Required, s.ForceNew)
		}
	}
	for _, field := range []string{"name", "description"} {
		s := r.Schema[field]
		if s == nil || !s.Optional || !s.Computed {
			t.Errorf("%s should be Optional+Computed", field)
		}
	}
	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required")
	}
}
