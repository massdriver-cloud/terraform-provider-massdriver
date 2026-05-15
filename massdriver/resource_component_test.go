package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/components"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakeComponents struct {
	getResp, addResp, updateResp, removeResp *components.Component
	getErr, addErr, updateErr, removeErr     error

	getID        string
	addProjectID string
	addInput     components.AddInput
	updateID     string
	updateInput  components.UpdateInput
	removeID     string

	getCalls, addCalls, updateCalls, removeCalls int
}

func (f *fakeComponents) Get(_ context.Context, id string) (*components.Component, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeComponents) Add(_ context.Context, projectID string, input components.AddInput) (*components.Component, error) {
	f.addProjectID = projectID
	f.addInput = input
	f.addCalls++
	return f.addResp, f.addErr
}
func (f *fakeComponents) Update(_ context.Context, id string, input components.UpdateInput) (*components.Component, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeComponents) Remove(_ context.Context, id string) (*components.Component, error) {
	f.removeID = id
	f.removeCalls++
	return f.removeResp, f.removeErr
}

func TestResourceComponentCreate(t *testing.T) {
	resp := &components.Component{
		ID:          "ecomm-db",
		Name:        "Primary Database",
		Description: "production DB",
		Attributes:  map[string]any{"tier": "critical"},
		OciRepo:     &types.OciRepo{Name: "aws-rds-cluster"},
		Project:     &types.Project{ID: "ecomm"},
	}
	fake := &fakeComponents{addResp: resp, getResp: resp}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		"name":        "Primary Database",
		"description": "production DB",
		"attributes":  map[string]any{"tier": "critical"},
	})

	if diags := resourceComponentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm-db" {
		t.Errorf("got id %q, want ecomm-db", rd.Id())
	}
	if fake.addProjectID != "ecomm" {
		t.Errorf("got Add projectID %q, want ecomm", fake.addProjectID)
	}
	in := fake.addInput
	if in.OciRepoName != "aws-rds-cluster" {
		t.Errorf("got input.OciRepoName %q, want aws-rds-cluster", in.OciRepoName)
	}
	if in.ID != "db" || in.Name != "Primary Database" || in.Description != "production DB" {
		t.Errorf("got input %+v", in)
	}
	if in.Attributes["tier"] != "critical" {
		t.Errorf("got Attributes %v", in.Attributes)
	}
	if rd.Get("bundle_name").(string) != "aws-rds-cluster" {
		t.Errorf("bundle_name should round-trip via Read; got %q", rd.Get("bundle_name"))
	}
}

func TestResourceComponentReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Components: &fakeComponents{
		getErr: fmt.Errorf("get component gone: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
	})
	rd.SetId("ecomm-db")

	if diags := resourceComponentRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared when component vanishes server-side, got %q", rd.Id())
	}
}

// Importing recovers project_id from the platform record (the SDK's Get
// returns the parent project), and identifier by stripping the project
// prefix.
func TestResourceComponentReadRecoversProjectIDOnImport(t *testing.T) {
	pc := &ProviderClient{Components: &fakeComponents{
		getResp: &components.Component{
			ID:      "ecomm-db",
			Name:    "Primary Database",
			OciRepo: &types.OciRepo{Name: "aws-rds-cluster"},
			Project: &types.Project{ID: "ecomm"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm-db")

	if diags := resourceComponentRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("project_id").(string) != "ecomm" {
		t.Errorf("got project_id %q, want ecomm", rd.Get("project_id"))
	}
	if rd.Get("identifier").(string) != "db" {
		t.Errorf("got identifier %q, want db", rd.Get("identifier"))
	}
	if rd.Get("bundle_name").(string) != "aws-rds-cluster" {
		t.Errorf("got bundle_name %q, want aws-rds-cluster", rd.Get("bundle_name"))
	}
}

func TestResourceComponentUpdate(t *testing.T) {
	resp := &components.Component{
		ID:          "ecomm-db",
		Name:        "Renamed",
		Description: "updated",
		OciRepo:     &types.OciRepo{Name: "aws-rds-cluster"},
		Project:     &types.Project{ID: "ecomm"},
	}
	fake := &fakeComponents{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("ecomm-db")

	if diags := resourceComponentUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "ecomm-db" {
		t.Errorf("got updateID %q, want ecomm-db", fake.updateID)
	}
	in := fake.updateInput
	if in.Name != "Renamed" || in.Description != "updated" {
		t.Errorf("got input %+v", in)
	}
}

func TestResourceComponentDelete(t *testing.T) {
	fake := &fakeComponents{removeResp: &components.Component{ID: "ecomm-db"}}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm-db")

	if diags := resourceComponentDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.removeID != "ecomm-db" {
		t.Errorf("got removeID %q, want ecomm-db", fake.removeID)
	}
}

func TestResourceComponentDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeComponents{removeErr: fmt.Errorf("remove component: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceComponentDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceComponentCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeComponents{addErr: fmt.Errorf("add component: id already exists in project")}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
	})

	diags := resourceComponentCreate(t.Context(), rd, pc)
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

func TestResourceComponentCreateDefaultsNameToIdentifier(t *testing.T) {
	resp := &components.Component{
		ID:      "ecomm-db",
		Name:    "db",
		OciRepo: &types.OciRepo{Name: "aws-rds-cluster"},
		Project: &types.Project{ID: "ecomm"},
	}
	fake := &fakeComponents{addResp: resp, getResp: resp}
	pc := &ProviderClient{Components: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		// name omitted
	})

	if diags := resourceComponentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.addInput.Name != "db" {
		t.Errorf("got input.Name %q, want db (defaulted from identifier)", fake.addInput.Name)
	}
}

func TestResourceComponentIgnoresNameDriftWhenConfigUnset(t *testing.T) {
	r := resourceComponent()

	state := &terraform.InstanceState{
		ID: "ecomm-db",
		Attributes: map[string]string{
			"id":          "ecomm-db",
			"identifier":  "db",
			"project_id":  "ecomm",
			"bundle_name": "aws-rds-cluster",
			"name":        "Console Edit",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
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

func TestResourceComponentSurfacesAttributesDrift(t *testing.T) {
	r := resourceComponent()

	state := &terraform.InstanceState{
		ID: "ecomm-db",
		Attributes: map[string]string{
			"id":              "ecomm-db",
			"identifier":      "db",
			"project_id":      "ecomm",
			"bundle_name":     "aws-rds-cluster",
			"attributes.%":    "1",
			"attributes.tier": "low",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		"attributes":  map[string]any{"tier": "critical"},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected attributes drift to surface as a diff")
	}
	if attr := diff.Attributes["attributes.tier"]; attr == nil {
		t.Error("expected diff on attributes.tier")
	} else if attr.Old != "low" || attr.New != "critical" {
		t.Errorf("got attributes.tier diff %+v, want Old=low New=critical", attr)
	}
}

func TestResourceComponentSchema(t *testing.T) {
	r := resourceComponent()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	for _, field := range []string{"identifier", "project_id", "bundle_name"} {
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
