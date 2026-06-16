package massdriver

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/components"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

// fakeComponentLinks satisfies componentLinksAPI. Records every call so tests
// can assert against the AddLink input the resource constructed.
type fakeComponentLinks struct {
	addResp, removeResp *components.Link
	addErr, removeErr   error

	addInput components.AddLinkInput
	removeID string

	addCalls, removeCalls int
}

func (f *fakeComponentLinks) AddLink(_ context.Context, input components.AddLinkInput) (*components.Link, error) {
	f.addInput = input
	f.addCalls++
	return f.addResp, f.addErr
}
func (f *fakeComponentLinks) RemoveLink(_ context.Context, linkID string) (*components.Link, error) {
	f.removeID = linkID
	f.removeCalls++
	return f.removeResp, f.removeErr
}

func TestResourceComponentLinkCreate(t *testing.T) {
	resp := &components.Link{
		ID:        "link-1",
		FromField: "network",
		ToField:   "network",
		FromComponent: &types.Component{
			ID:      "ecomm-network",
			Project: &types.Project{ID: "ecomm"},
		},
		ToComponent: &types.Component{ID: "ecomm-db"},
	}
	fake := &fakeComponentLinks{addResp: resp}
	pc := &ProviderClient{ComponentLinks: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"from_component_id": "ecomm-network",
		"from_field":        "network",
		"from_version":      "latest+dev",
		"to_component_id":   "ecomm-db",
		"to_field":          "network",
		"to_version":        "latest+dev",
	})

	if diags := resourceComponentLinkCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "link-1" {
		t.Errorf("got id %q, want link-1", rd.Id())
	}
	if got := rd.Get("project_id").(string); got != "ecomm" {
		t.Errorf("got project_id %q, want ecomm (derived from FromComponent.Project)", got)
	}
	in := fake.addInput
	if in.FromComponentID != "ecomm-network" || in.FromField != "network" || in.FromVersion != "latest+dev" {
		t.Errorf("got addInput from-side %+v", in)
	}
	if in.ToComponentID != "ecomm-db" || in.ToField != "network" || in.ToVersion != "latest+dev" {
		t.Errorf("got addInput to-side %+v", in)
	}
}

// If the AddLink response doesn't include the project embed (older SDK or
// trimmed GraphQL selection), Create falls back to a direct components.Get.
func TestResourceComponentLinkCreateFallsBackToComponentsGet(t *testing.T) {
	links := &fakeComponentLinks{addResp: &components.Link{
		ID:            "link-1",
		FromComponent: &types.Component{ID: "ecomm-network"}, // Project intentionally nil
	}}
	comps := &fakeComponents{getResp: &components.Component{
		ID:      "ecomm-network",
		Project: &types.Project{ID: "ecomm"},
	}}
	pc := &ProviderClient{ComponentLinks: links, Components: comps}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"from_component_id": "ecomm-network",
		"from_field":        "network",
		"to_component_id":   "ecomm-db",
		"to_field":          "network",
	})

	if diags := resourceComponentLinkCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := rd.Get("project_id").(string); got != "ecomm" {
		t.Errorf("got project_id %q, want ecomm (from fallback Get)", got)
	}
	if comps.getID != "ecomm-network" {
		t.Errorf("fallback should Get from_component_id, got %q", comps.getID)
	}
}

func TestResourceComponentLinkCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeComponentLinks{addErr: fmt.Errorf("addLink: field `cromulence` does not exist on bundle version 1.0.0")}
	pc := &ProviderClient{ComponentLinks: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{
		"from_component_id": "ecomm-network",
		"from_field":        "cromulence",
		"to_component_id":   "ecomm-db",
		"to_field":          "network",
	})

	diags := resourceComponentLinkCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceComponentLinkRead(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{
		getResp: &types.Project{
			ID: "ecomm",
			Links: []types.Link{
				{ID: "other-link"},
				{ID: "link-1"},
				{ID: "another-link"},
			},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-1")
	if err := rd.Set("project_id", "ecomm"); err != nil {
		t.Fatalf("seed project_id: %v", err)
	}

	if diags := resourceComponentLinkRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "link-1" {
		t.Errorf("ID should be preserved when link found, got %q", rd.Id())
	}
}

func TestResourceComponentLinkReadClearsWhenProjectGone(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{
		getErr: fmt.Errorf("project lookup: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-1")
	if err := rd.Set("project_id", "gone"); err != nil {
		t.Fatalf("seed project_id: %v", err)
	}

	if diags := resourceComponentLinkRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared when project is gone, got %q", rd.Id())
	}
}

func TestResourceComponentLinkReadClearsWhenLinkMissing(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{
		getResp: &types.Project{
			ID:    "ecomm",
			Links: []types.Link{{ID: "different-link"}},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-1")
	if err := rd.Set("project_id", "ecomm"); err != nil {
		t.Fatalf("seed project_id: %v", err)
	}

	if diags := resourceComponentLinkRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared when link missing from project, got %q", rd.Id())
	}
}

// Older state migrated forward might lack project_id (the field was added
// later). Read should silently no-op rather than failing the apply.
func TestResourceComponentLinkReadNoOpWhenProjectIDEmpty(t *testing.T) {
	pc := &ProviderClient{Projects: &fakeProjects{}}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-1")
	// project_id deliberately not set.

	if diags := resourceComponentLinkRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "link-1" {
		t.Errorf("ID should be preserved when project_id is empty, got %q", rd.Id())
	}
}

func TestResourceComponentLinkDelete(t *testing.T) {
	fake := &fakeComponentLinks{removeResp: &components.Link{ID: "link-1"}}
	pc := &ProviderClient{ComponentLinks: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("link-1")

	if diags := resourceComponentLinkDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.removeID != "link-1" {
		t.Errorf("got removeID %q, want link-1", fake.removeID)
	}
}

func TestResourceComponentLinkDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeComponentLinks{removeErr: fmt.Errorf("remove link: %w", gql.ErrNotFound)}
	pc := &ProviderClient{ComponentLinks: fake}

	rd := schema.TestResourceDataRaw(t, resourceComponentLink().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceComponentLinkDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

// The version fields are inputs to AddLink only — the server discards them
// after validation. After Create, any HCL change to from_version / to_version
// should be suppressed and produce no plan diff.
func TestResourceComponentLinkVersionDiffSuppressedAfterCreate(t *testing.T) {
	r := resourceComponentLink()

	state := &terraform.InstanceState{
		ID: "link-1",
		Attributes: map[string]string{
			"id":                "link-1",
			"from_component_id": "ecomm-network",
			"from_field":        "network",
			"from_version":      "latest+dev",
			"to_component_id":   "ecomm-db",
			"to_field":          "network",
			"to_version":        "latest+dev",
			"project_id":        "ecomm",
		},
	}
	// User edits HCL to pin a different version after the fact. This is the
	// kind of edit the schema deliberately makes inert.
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"from_component_id": "ecomm-network",
		"from_field":        "network",
		"from_version":      "2.0.0",
		"to_component_id":   "ecomm-db",
		"to_field":          "network",
		"to_version":        "2.0.0",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff != nil && !diff.Empty() {
		for k, attr := range diff.Attributes {
			if k == "from_version" || k == "to_version" {
				t.Errorf("expected no diff on %s after Create; got %+v", k, attr)
			}
		}
	}
}

// Field renames should still trigger destroy+recreate, because the link's
// identity IS (from_component, from_field, to_component, to_field). This is
// the case where the user's right move is actually to declare a new
// resource block, but if they DO mutate the existing one, terraform should
// force a replacement rather than try to update in place.
func TestResourceComponentLinkFieldRenameForcesNew(t *testing.T) {
	r := resourceComponentLink()

	state := &terraform.InstanceState{
		ID: "link-1",
		Attributes: map[string]string{
			"id":                "link-1",
			"from_component_id": "ecomm-network",
			"from_field":        "network",
			"to_component_id":   "ecomm-db",
			"to_field":          "database", // old name
			"project_id":        "ecomm",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"from_component_id": "ecomm-network",
		"from_field":        "network",
		"to_component_id":   "ecomm-db",
		"to_field":          "postgres", // renamed in newer bundle
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected a diff when to_field changes")
	}
	if attr := diff.Attributes["to_field"]; attr == nil || !attr.RequiresNew {
		t.Errorf("to_field change should be RequiresNew; got %+v", attr)
	}
}

func TestResourceComponentLinkSchema(t *testing.T) {
	r := resourceComponentLink()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	for _, field := range []string{"from_component_id", "from_field", "to_component_id", "to_field"} {
		s := r.Schema[field]
		if s == nil {
			t.Errorf("expected %s in schema", field)
			continue
		}
		if !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew; got Required=%v ForceNew=%v", field, s.Required, s.ForceNew)
		}
	}

	for _, field := range []string{"from_version", "to_version"} {
		s := r.Schema[field]
		if s == nil {
			t.Errorf("expected %s in schema", field)
			continue
		}
		if !s.Optional || s.ForceNew {
			t.Errorf("%s should be Optional and NOT ForceNew; got Optional=%v ForceNew=%v", field, s.Optional, s.ForceNew)
		}
		if s.Default != "latest+dev" {
			t.Errorf("%s default should be latest+dev; got %v", field, s.Default)
		}
		if s.DiffSuppressFunc == nil {
			t.Errorf("%s should have a DiffSuppressFunc to ignore post-Create edits", field)
		}
	}

	if pid := r.Schema["project_id"]; pid == nil || !pid.Computed {
		t.Error("project_id should be Computed")
	}
}
