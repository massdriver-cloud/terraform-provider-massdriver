package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceBundleRepositoryCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createOciRepo": {
			"data": map[string]any{
				"createOciRepo": map[string]any{
					"result": map[string]any{
						"id":           "aws-aurora-postgres",
						"name":         "aws-aurora-postgres",
						"reference":    "api.massdriver.cloud/acme/aws-aurora-postgres",
						"artifactType": "application/vnd.massdriver.bundle.v1+json",
						"attributes":   map[string]any{"team": "platform"},
					},
					"successful": true,
				},
			},
		},
		"getOciRepo": {
			"data": map[string]any{
				"ociRepo": map[string]any{
					"id":           "aws-aurora-postgres",
					"name":         "aws-aurora-postgres",
					"reference":    "api.massdriver.cloud/acme/aws-aurora-postgres",
					"artifactType": "application/vnd.massdriver.bundle.v1+json",
					"attributes":   map[string]any{"team": "platform"},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{
		"name":       "aws-aurora-postgres",
		"attributes": map[string]any{"team": "platform"},
	})

	if diags := resourceBundleRepositoryCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "aws-aurora-postgres" {
		t.Errorf("got id %q, want aws-aurora-postgres", rd.Id())
	}

	createReq := rec.FindRequest("createOciRepo")
	if createReq == nil {
		t.Fatal("createOciRepo was not called")
	}
	input, _ := gqlmock.Variables(createReq)["input"].(map[string]any)
	if input["id"] != "aws-aurora-postgres" {
		t.Errorf("got input.id %v, want aws-aurora-postgres", input["id"])
	}
	// massdriver_bundle_repository hardcodes BUNDLE — pinning the wire shape
	// so a future regression that drops the field is caught here.
	if input["artifactType"] != "BUNDLE" {
		t.Errorf("got input.artifactType %v, want BUNDLE", input["artifactType"])
	}
	if input["attributes"] != `{"team":"platform"}` {
		t.Errorf("got input.attributes %v, want JSON-encoded team=platform", input["attributes"])
	}

	// Read populates the computed reference + artifact_type in state.
	if rd.Get("reference").(string) != "api.massdriver.cloud/acme/aws-aurora-postgres" {
		t.Errorf("got reference %q", rd.Get("reference"))
	}
	if rd.Get("artifact_type").(string) != "application/vnd.massdriver.bundle.v1+json" {
		t.Errorf("got artifact_type %q", rd.Get("artifact_type"))
	}
}

// Empty attributes must be omitted from the wire (server rejects an explicit
// `attributes: null` for the JSON scalar).
func TestResourceBundleRepositoryCreateOmitsEmptyAttributes(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createOciRepo": {
			"data": map[string]any{
				"createOciRepo": map[string]any{
					"result":     map[string]any{"id": "aws-aurora-postgres", "name": "aws-aurora-postgres"},
					"successful": true,
				},
			},
		},
		"getOciRepo": {
			"data": map[string]any{
				"ociRepo": map[string]any{"id": "aws-aurora-postgres", "name": "aws-aurora-postgres"},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{
		"name":       "aws-aurora-postgres",
		"attributes": map[string]any{},
	})

	if diags := resourceBundleRepositoryCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createOciRepo"))["input"].(map[string]any)
	if _, present := input["attributes"]; present {
		t.Errorf("attributes should be omitted from wire when empty, got %v", input["attributes"])
	}
}

func TestResourceBundleRepositoryCreatePropagatesAPIFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"createOciRepo": {
			"data": map[string]any{
				"createOciRepo": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "validation", "field": "attributes", "message": "key 'md-id' is reserved"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{
		"name":       "bad-repo",
		"attributes": map[string]any{"md-id": "nope"},
	})

	diags := resourceBundleRepositoryCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from failed mutation")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceBundleRepositoryRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getOciRepo": {
			"data": map[string]any{
				"ociRepo": map[string]any{
					"id":           "aws-aurora-postgres",
					"name":         "aws-aurora-postgres",
					"reference":    "api.massdriver.cloud/acme/aws-aurora-postgres",
					"artifactType": "application/vnd.massdriver.bundle.v1+json",
					"attributes":   map[string]any{"team": "from-server"},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{})
	rd.SetId("aws-aurora-postgres")

	if diags := resourceBundleRepositoryRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "aws-aurora-postgres" {
		t.Errorf("got name %q", rd.Get("name"))
	}
	if attrs := rd.Get("attributes").(map[string]any); attrs["team"] != "from-server" {
		t.Errorf("got attributes %v, want team=from-server", attrs)
	}
}

func TestResourceBundleRepositoryUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateOciRepo": {
			"data": map[string]any{
				"updateOciRepo": map[string]any{
					"result": map[string]any{
						"id":         "aws-aurora-postgres",
						"name":       "aws-aurora-postgres",
						"attributes": map[string]any{"team": "infra"},
					},
					"successful": true,
				},
			},
		},
		"getOciRepo": {
			"data": map[string]any{
				"ociRepo": map[string]any{
					"id":         "aws-aurora-postgres",
					"name":       "aws-aurora-postgres",
					"attributes": map[string]any{"team": "infra"},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{
		"name":       "aws-aurora-postgres",
		"attributes": map[string]any{"team": "infra"},
	})
	rd.SetId("aws-aurora-postgres")

	if diags := resourceBundleRepositoryUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateOciRepo")
	if updateReq == nil {
		t.Fatal("updateOciRepo was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "aws-aurora-postgres" {
		t.Errorf("got id %v", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	// Update only sends attributes — name/artifactType are immutable.
	if input["attributes"] != `{"team":"infra"}` {
		t.Errorf("got input.attributes %v, want JSON-encoded team=infra", input["attributes"])
	}
}

func TestResourceBundleRepositoryDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteOciRepo": {
			"data": map[string]any{
				"deleteOciRepo": map[string]any{
					"result":     map[string]any{"id": "aws-aurora-postgres", "name": "aws-aurora-postgres"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{})
	rd.SetId("aws-aurora-postgres")

	if diags := resourceBundleRepositoryDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deleteOciRepo")); vars["id"] != "aws-aurora-postgres" {
		t.Errorf("got id %v", vars["id"])
	}
}

// The server refuses delete when the repo has published versions. The
// resource must surface that as an error so terraform doesn't silently drop
// the resource from state.
func TestResourceBundleRepositoryDeletePropagatesConflict(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"deleteOciRepo": {
			"data": map[string]any{
				"deleteOciRepo": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "conflict", "field": "id", "message": "repository has published versions"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceBundleRepository().Schema, map[string]any{})
	rd.SetId("aws-aurora-postgres")

	diags := resourceBundleRepositoryDelete(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error rejecting delete of repo with versions")
	}
	if rd.Id() != "aws-aurora-postgres" {
		t.Errorf("ID should not be cleared on failed delete, got %q", rd.Id())
	}
}

// Drift on attributes MUST surface — they drive ABAC policy. Same pattern as
// project/environment/component.
func TestResourceBundleRepositorySurfacesAttributesDrift(t *testing.T) {
	r := resourceBundleRepository()

	state := &terraform.InstanceState{
		ID: "aws-aurora-postgres",
		Attributes: map[string]string{
			"id":              "aws-aurora-postgres",
			"name":            "aws-aurora-postgres",
			"attributes.%":    "1",
			"attributes.team": "infra", // drifted via console
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"name":       "aws-aurora-postgres",
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

func TestResourceBundleRepositorySchema(t *testing.T) {
	r := resourceBundleRepository()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if name := r.Schema["name"]; name == nil || !name.Required || !name.ForceNew || name.ValidateFunc == nil {
		t.Error("name should be Required+ForceNew with a ValidateFunc")
	}
	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required (drift always surfaces)")
	}
	for _, field := range []string{"reference", "artifact_type"} {
		s := r.Schema[field]
		if s == nil || !s.Computed || s.Required || s.Optional {
			t.Errorf("%s should be Computed-only", field)
		}
	}
}

func TestResourceBundleRepositoryNameValidation(t *testing.T) {
	name := resourceBundleRepository().Schema["name"]
	cases := []struct {
		value string
		valid bool
	}{
		{"aws-aurora-postgres", true},
		{"a", true},
		{"snake_case_repo", true},
		{"with-numbers-123", true},
		{"fiftythreecharacters01234567890123456789012345678901", true},  // 52 chars
		{"fiftythreecharacters012345678901234567890123456789012", true}, // 53 chars
		{"fiftyfourcharacters0123456789012345678901234567890123x", false},
		{"", false},
		{"UPPERCASE", false},
		{"with.dot", false},
		{"with space", false},
		{"with/slash", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			_, errs := name.ValidateFunc(tc.value, "name")
			if tc.valid && len(errs) > 0 {
				t.Errorf("expected %q to validate, got errors: %v", tc.value, errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("expected %q to be rejected, got no errors", tc.value)
			}
		})
	}
}
