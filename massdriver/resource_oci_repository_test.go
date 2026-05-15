package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/ocirepos"
)

type fakeOciRepos struct {
	getResp, createResp, updateResp, deleteResp *ocirepos.OciRepo
	getErr, createErr, updateErr, deleteErr     error

	getID       string
	createInput ocirepos.CreateInput
	updateID    string
	updateInput ocirepos.UpdateInput
	deleteID    string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeOciRepos) Get(_ context.Context, id string) (*ocirepos.OciRepo, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeOciRepos) Create(_ context.Context, input ocirepos.CreateInput) (*ocirepos.OciRepo, error) {
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeOciRepos) Update(_ context.Context, id string, input ocirepos.UpdateInput) (*ocirepos.OciRepo, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeOciRepos) Delete(_ context.Context, id string) (*ocirepos.OciRepo, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceOciRepositoryCreate(t *testing.T) {
	resp := &ocirepos.OciRepo{
		ID:           "aws-aurora-postgres",
		Name:         "aws-aurora-postgres",
		Reference:    "api.massdriver.cloud/test-org/aws-aurora-postgres",
		ArtifactType: ocirepos.ArtifactTypeBundle,
		Attributes:   map[string]any{"team": "platform"},
	}
	fake := &fakeOciRepos{createResp: resp, getResp: resp}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{
		"name":          "aws-aurora-postgres",
		"artifact_type": "BUNDLE",
		"attributes":    map[string]any{"team": "platform"},
	})

	if diags := resourceOciRepositoryCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "aws-aurora-postgres" {
		t.Errorf("got id %q, want aws-aurora-postgres", rd.Id())
	}
	in := fake.createInput
	if in.ID != "aws-aurora-postgres" {
		t.Errorf("got ID %q, want aws-aurora-postgres", in.ID)
	}
	// The user's HCL string value is cast to the SDK's typed enum.
	if in.ArtifactType != ocirepos.ArtifactTypeBundle {
		t.Errorf("got ArtifactType %v, want %v", in.ArtifactType, ocirepos.ArtifactTypeBundle)
	}
	if in.Attributes["team"] != "platform" {
		t.Errorf("got Attributes %v, want team=platform", in.Attributes)
	}

	// Read populates the computed reference + (no longer-computed) artifact_type passthrough.
	if rd.Get("reference").(string) != resp.Reference {
		t.Errorf("got reference %q, want %q", rd.Get("reference"), resp.Reference)
	}
	if rd.Get("artifact_type").(string) != string(ocirepos.ArtifactTypeBundle) {
		t.Errorf("got artifact_type %q", rd.Get("artifact_type"))
	}
}

func TestResourceOciRepositoryCreatePassesEmptyAttributes(t *testing.T) {
	resp := &ocirepos.OciRepo{ID: "x", Name: "x", ArtifactType: ocirepos.ArtifactTypeBundle}
	fake := &fakeOciRepos{createResp: resp, getResp: resp}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{
		"name":          "x",
		"artifact_type": "BUNDLE",
		"attributes":    map[string]any{},
	})

	if diags := resourceOciRepositoryCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(fake.createInput.Attributes) != 0 {
		t.Errorf("expected empty Attributes map, got %v", fake.createInput.Attributes)
	}
}

// The provider passes any artifact_type value through to the SDK; if the
// server rejects an unknown type (e.g., a typo or a type not yet supported),
// the error bubbles up verbatim. Intentional design — no client-side
// allowlist that would lag the platform.
func TestResourceOciRepositoryCreatePassesArtifactTypeVerbatim(t *testing.T) {
	fake := &fakeOciRepos{createErr: fmt.Errorf("create oci repo: artifact type RESOURCE_TYPE is not supported in this organization")}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{
		"name":          "my-resource-types",
		"artifact_type": "RESOURCE_TYPE",
	})

	diags := resourceOciRepositoryCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from server-side artifact-type rejection")
	}
	// Confirm the provider sent the value verbatim — no validation rewriting.
	if got := fake.createInput.ArtifactType; got != ocirepos.ArtifactType("RESOURCE_TYPE") {
		t.Errorf("got ArtifactType %q, want RESOURCE_TYPE (sent verbatim to SDK)", got)
	}
	if !strings.Contains(diags[0].Summary, "RESOURCE_TYPE is not supported") {
		t.Errorf("server-side rejection should propagate; got %q", diags[0].Summary)
	}
}

func TestResourceOciRepositoryCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeOciRepos{createErr: fmt.Errorf("create oci repo: name already exists")}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{
		"name":          "aws-aurora-postgres",
		"artifact_type": "BUNDLE",
	})

	diags := resourceOciRepositoryCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "name already exists") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceOciRepositoryRead(t *testing.T) {
	pc := &ProviderClient{OciRepos: &fakeOciRepos{
		getResp: &ocirepos.OciRepo{
			ID:           "aws-aurora-postgres",
			Name:         "aws-aurora-postgres",
			Reference:    "api.massdriver.cloud/test-org/aws-aurora-postgres",
			ArtifactType: ocirepos.ArtifactTypeBundle,
			Attributes:   map[string]any{"team": "platform"},
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{})
	rd.SetId("aws-aurora-postgres")

	if diags := resourceOciRepositoryRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "aws-aurora-postgres" {
		t.Errorf("got name %q", rd.Get("name"))
	}
	if rd.Get("reference").(string) != "api.massdriver.cloud/test-org/aws-aurora-postgres" {
		t.Errorf("got reference %q", rd.Get("reference"))
	}
	if rd.Get("artifact_type").(string) != "BUNDLE" {
		t.Errorf("got artifact_type %q, want BUNDLE", rd.Get("artifact_type"))
	}
	if attrs := rd.Get("attributes").(map[string]any); attrs["team"] != "platform" {
		t.Errorf("got attributes %v", attrs)
	}
}

func TestResourceOciRepositoryReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{OciRepos: &fakeOciRepos{
		getErr: fmt.Errorf("get oci repo: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceOciRepositoryRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceOciRepositoryUpdate(t *testing.T) {
	resp := &ocirepos.OciRepo{
		ID:           "aws-aurora-postgres",
		Name:         "aws-aurora-postgres",
		ArtifactType: ocirepos.ArtifactTypeBundle,
		Attributes:   map[string]any{"team": "infra"},
	}
	fake := &fakeOciRepos{updateResp: resp, getResp: resp}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{
		"name":          "aws-aurora-postgres",
		"artifact_type": "BUNDLE",
		"attributes":    map[string]any{"team": "infra"},
	})
	rd.SetId("aws-aurora-postgres")

	if diags := resourceOciRepositoryUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "aws-aurora-postgres" {
		t.Errorf("got updateID %q, want aws-aurora-postgres", fake.updateID)
	}
	if fake.updateInput.Attributes["team"] != "infra" {
		t.Errorf("got updateInput.Attributes %v", fake.updateInput.Attributes)
	}
}

func TestResourceOciRepositoryDelete(t *testing.T) {
	fake := &fakeOciRepos{deleteResp: &ocirepos.OciRepo{ID: "x"}}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{})
	rd.SetId("x")

	if diags := resourceOciRepositoryDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared after delete, got %q", rd.Id())
	}
	if fake.deleteID != "x" {
		t.Errorf("got deleteID %q, want x", fake.deleteID)
	}
}

func TestResourceOciRepositoryDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeOciRepos{deleteErr: fmt.Errorf("delete oci repo: %w", gql.ErrNotFound)}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceOciRepositoryDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceOciRepositoryDeletePropagatesConflict(t *testing.T) {
	fake := &fakeOciRepos{deleteErr: fmt.Errorf("delete oci repo: repo still has 3 published versions")}
	pc := &ProviderClient{OciRepos: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepository().Schema, map[string]any{})
	rd.SetId("aws-aurora-postgres")

	diags := resourceOciRepositoryDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from conflicted delete, got none")
	}
	if !strings.Contains(diags[0].Summary, "still has 3 published versions") {
		t.Errorf("upstream error %q should be surfaced verbatim", diags[0].Summary)
	}
}

func TestResourceOciRepositorySurfacesAttributesDrift(t *testing.T) {
	r := resourceOciRepository()

	state := &terraform.InstanceState{
		ID: "aws-aurora-postgres",
		Attributes: map[string]string{
			"id":              "aws-aurora-postgres",
			"name":            "aws-aurora-postgres",
			"artifact_type":   "BUNDLE",
			"attributes.%":    "1",
			"attributes.team": "infra",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"name":          "aws-aurora-postgres",
		"artifact_type": "BUNDLE",
		"attributes":    map[string]any{"team": "platform"},
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

// Changing artifact_type forces a destroy+recreate — repositories cannot be
// retyped in place.
func TestResourceOciRepositoryArtifactTypeIsForceNew(t *testing.T) {
	r := resourceOciRepository()

	state := &terraform.InstanceState{
		ID: "my-repo",
		Attributes: map[string]string{
			"id":              "my-repo",
			"name":            "my-repo",
			"artifact_type":   "BUNDLE",
			"attributes.%":    "0",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"name":          "my-repo",
		"artifact_type": "RESOURCE_TYPE",
		"attributes":    map[string]any{},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected a diff because artifact_type changed")
	}
	if attr := diff.Attributes["artifact_type"]; attr == nil || !attr.RequiresNew {
		t.Errorf("artifact_type change should force destroy+recreate; got %+v", attr)
	}
}

func TestResourceOciRepositorySchema(t *testing.T) {
	r := resourceOciRepository()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if name := r.Schema["name"]; name == nil || !name.Required || !name.ForceNew {
		t.Error("name should be Required+ForceNew")
	}
	if at := r.Schema["artifact_type"]; at == nil || !at.Required || !at.ForceNew {
		t.Error("artifact_type should be Required+ForceNew (immutable, server-validated)")
	}
	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required")
	}
	if ref := r.Schema["reference"]; ref == nil || !ref.Computed {
		t.Error("reference should be Computed")
	}
}

func TestResourceOciRepositoryNameValidation(t *testing.T) {
	name := resourceOciRepository().Schema["name"]
	if name.ValidateFunc == nil {
		t.Fatal("name has no ValidateFunc")
	}

	cases := []struct {
		value string
		valid bool
	}{
		{"aws-aurora-postgres", true},
		{"a", true},
		{"with_underscores", true},
		{"with-dashes-and_underscores", true},
		{"a1b2", true},
		{strings.Repeat("a", 53), true},  // exactly 53 chars
		{strings.Repeat("a", 54), false}, // too long
		{"", false},
		{"Upper", false},
		{"has space", false},
		{"has.dot", false},
		{"has/slash", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			_, errs := name.ValidateFunc(tc.value, "name")
			if tc.valid && len(errs) > 0 {
				t.Errorf("expected %q to be valid, got errors: %v", tc.value, errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("expected %q to be rejected, got no errors", tc.value)
			}
		})
	}
}
