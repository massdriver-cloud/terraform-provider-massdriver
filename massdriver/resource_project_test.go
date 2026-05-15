package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/projects"
)

// fakeProjects records every call for assertion and returns whatever canned
// response the test wires in. Satisfies projectsAPI.
type fakeProjects struct {
	getResp, createResp, updateResp, deleteResp *projects.Project
	getErr, createErr, updateErr, deleteErr     error

	getID       string
	createInput projects.CreateInput
	updateID    string
	updateInput projects.UpdateInput
	deleteID    string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeProjects) Get(_ context.Context, id string) (*projects.Project, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeProjects) Create(_ context.Context, input projects.CreateInput) (*projects.Project, error) {
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeProjects) Update(_ context.Context, id string, input projects.UpdateInput) (*projects.Project, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeProjects) Delete(_ context.Context, id string) (*projects.Project, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceProjectCreate(t *testing.T) {
	resp := &projects.Project{
		ID:          "ecomm",
		Name:        "Ecomm Project",
		Description: "the e-commerce app",
		Attributes:  map[string]any{"team": "platform"},
	}
	fake := &fakeProjects{createResp: resp, getResp: resp}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier":  "ecomm",
		"name":        "Ecomm Project",
		"description": "the e-commerce app",
		"attributes":  map[string]any{"team": "platform"},
	})

	if diags := resourceProjectCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm" {
		t.Errorf("got id %q, want ecomm", rd.Id())
	}
	if got := rd.Get("identifier").(string); got != "ecomm" {
		t.Errorf("got identifier %q, want ecomm", got)
	}
	if got := rd.Get("name").(string); got != "Ecomm Project" {
		t.Errorf("got name %q, want Ecomm Project", got)
	}
	if got := rd.Get("description").(string); got != "the e-commerce app" {
		t.Errorf("got description %q, want %q", got, "the e-commerce app")
	}

	if fake.createCalls != 1 {
		t.Fatalf("Create called %d times, want 1", fake.createCalls)
	}
	in := fake.createInput
	if in.ID != "ecomm" {
		t.Errorf("got input.ID %q, want ecomm", in.ID)
	}
	if in.Name != "Ecomm Project" {
		t.Errorf("got input.Name %q, want Ecomm Project", in.Name)
	}
	if in.Description != "the e-commerce app" {
		t.Errorf("got input.Description %q", in.Description)
	}
	if in.Attributes["team"] != "platform" {
		t.Errorf("got input.Attributes %v, want team=platform", in.Attributes)
	}

	if fake.getCalls != 1 {
		t.Error("Read was not called after Create")
	}
	if attrs := rd.Get("attributes").(map[string]any); attrs["team"] != "platform" {
		t.Errorf("got attributes %v after Read, want team=platform", attrs)
	}
}

// Empty attributes map is passed through unchanged — SDK handles wire
// encoding. The provider's job is to convert HCL state → SDK input, not to
// fight the wire shape.
func TestResourceProjectCreatePassesEmptyAttributes(t *testing.T) {
	resp := &projects.Project{ID: "ecomm", Name: "Ecomm"}
	fake := &fakeProjects{createResp: resp, getResp: resp}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		"name":       "Ecomm",
		"attributes": map[string]any{},
	})

	if diags := resourceProjectCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(fake.createInput.Attributes) != 0 {
		t.Errorf("expected empty Attributes map, got %v", fake.createInput.Attributes)
	}
}

// Drift on attributes MUST surface — they drive permissions, so silent drift
// would be a security issue.
func TestResourceProjectSurfacesAttributesDrift(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":              "ecomm",
			"identifier":      "ecomm",
			"attributes.%":    "1",
			"attributes.team": "infra",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
		"attributes": map[string]any{"team": "platform"},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected attributes drift to surface as a diff")
	}
	if attr := diff.Attributes["attributes.team"]; attr == nil {
		t.Error("expected diff on attributes.team")
	} else if attr.Old != "infra" || attr.New != "platform" {
		t.Errorf("got attributes.team diff %+v, want Old=infra New=platform", attr)
	}
}

func TestResourceProjectCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeProjects{createErr: fmt.Errorf("create project: id already exists")}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		"name":       "Ecomm Project",
	})

	diags := resourceProjectCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
	if !strings.Contains(diags[0].Summary, "id already exists") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
}

func TestResourceProjectRead(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{
		getResp: &projects.Project{
			ID:          "ecomm",
			Name:        "Ecomm Project",
			Description: "from the server",
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("ecomm")

	if diags := resourceProjectRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("identifier").(string) != "ecomm" {
		t.Errorf("got identifier %q, want ecomm", rd.Get("identifier"))
	}
	if rd.Get("name").(string) != "Ecomm Project" {
		t.Errorf("got name %q, want Ecomm Project", rd.Get("name"))
	}
	if rd.Get("description").(string) != "from the server" {
		t.Errorf("got description %q", rd.Get("description"))
	}
}

// A `gql.ErrNotFound` from Get clears state so terraform plans a recreate.
func TestResourceProjectReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{
		getErr: fmt.Errorf("get project gone: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceProjectRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceProjectUpdate(t *testing.T) {
	resp := &projects.Project{ID: "ecomm", Name: "Renamed", Description: "updated"}
	fake := &fakeProjects{updateResp: resp, getResp: resp}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier":  "ecomm",
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("ecomm")

	if diags := resourceProjectUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.updateID != "ecomm" {
		t.Errorf("got updateID %q, want ecomm", fake.updateID)
	}
	in := fake.updateInput
	if in.Name != "Renamed" || in.Description != "updated" {
		t.Errorf("got input %+v, want name=Renamed description=updated", in)
	}
}

func TestResourceProjectDelete(t *testing.T) {
	fake := &fakeProjects{deleteResp: &projects.Project{ID: "ecomm"}}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("ecomm")

	if diags := resourceProjectDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared after delete, got %q", rd.Id())
	}
	if fake.deleteID != "ecomm" {
		t.Errorf("got deleteID %q, want ecomm", fake.deleteID)
	}
}

// Delete returning not-found is treated as success — the record is gone, which
// is what destroy wanted anyway.
func TestResourceProjectDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeProjects{deleteErr: fmt.Errorf("delete project: %w", gql.ErrNotFound)}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceProjectDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

// When the user omits `name`, the resource substitutes `identifier` so the API
// (which requires `name`) doesn't reject.
func TestResourceProjectCreateDefaultsNameToIdentifier(t *testing.T) {
	resp := &projects.Project{ID: "ecomm", Name: "ecomm"}
	fake := &fakeProjects{createResp: resp, getResp: resp}
	pc := &ProviderClient{Projects: fake}

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		// name and description deliberately omitted
	})

	if diags := resourceProjectCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.createInput.Name != "ecomm" {
		t.Errorf("got input.Name %q, want ecomm (defaulted from identifier)", fake.createInput.Name)
	}
	if fake.createInput.Description != "" {
		t.Errorf("got input.Description %q, want empty", fake.createInput.Description)
	}
}

// When the user omits name/description from config, terraform's diff resolver
// must keep whatever is currently in state — that's how console edits avoid
// being reverted on the next apply.
func TestResourceProjectIgnoresDriftWhenConfigUnset(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":          "ecomm",
			"identifier":  "ecomm",
			"name":        "Console Edit",
			"description": "added in the console",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
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

// Conversely, if the user DOES specify a value, drift surfaces as a real diff.
func TestResourceProjectShowsDriftWhenConfigSet(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":         "ecomm",
			"identifier": "ecomm",
			"name":       "Console Edit",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
		"name":       "Tracked By Terraform",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected a diff because config sets a value that differs from state")
	}
	if attr := diff.Attributes["name"]; attr == nil {
		t.Error("expected a diff entry for `name`")
	} else if attr.New != "Tracked By Terraform" || attr.Old != "Console Edit" {
		t.Errorf("got name diff %+v, want Old=Console Edit New=Tracked By Terraform", attr)
	}
}

func TestResourceProjectSchema(t *testing.T) {
	r := resourceProject()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	identifier := r.Schema["identifier"]
	if identifier == nil {
		t.Fatal("expected identifier attribute in schema")
	}
	if !identifier.Required || !identifier.ForceNew {
		t.Errorf("identifier should be Required+ForceNew, got Required=%v ForceNew=%v", identifier.Required, identifier.ForceNew)
	}
	if identifier.ValidateFunc == nil {
		t.Error("identifier should have a ValidateFunc enforcing the regex")
	}

	for _, field := range []string{"name", "description"} {
		s := r.Schema[field]
		if s == nil {
			t.Fatalf("expected %s attribute in schema", field)
		}
		if s.Required {
			t.Errorf("%s should not be Required", field)
		}
		if !s.Optional || !s.Computed {
			t.Errorf("%s should be Optional+Computed (got Optional=%v Computed=%v)", field, s.Optional, s.Computed)
		}
	}

	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required (drift always surfaces)")
	}

	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}

func TestResourceProjectIdentifierValidation(t *testing.T) {
	identifier := resourceProject().Schema["identifier"]
	if identifier.ValidateFunc == nil {
		t.Fatal("identifier has no ValidateFunc")
	}

	cases := []struct {
		value string
		valid bool
	}{
		{"ecomm", true},
		{"ec0mm", true},
		{"a", true},
		{"twentycharacterident", true},
		{"twentyonecharidentifier", false},
		{"", false},
		{"Ecomm", false},
		{"ec-omm", false},
		{"ec omm", false},
		{"ec_omm", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			_, errs := identifier.ValidateFunc(tc.value, "identifier")
			if tc.valid && len(errs) > 0 {
				t.Errorf("expected %q to be valid, got errors: %v", tc.value, errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("expected %q to be rejected, got no errors", tc.value)
			}
		})
	}
}
