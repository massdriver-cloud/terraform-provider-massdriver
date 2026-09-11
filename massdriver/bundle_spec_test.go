package massdriver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// writeSpec writes a massdriver.yaml containing the given YAML body.
func writeSpec(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "massdriver.yaml")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

const (
	// Modern layout: a `resources` map keyed by field.
	specResourcesBlock = `
resources:
  your_first_artifact:
    resource_type: chrissbx/getting-started-resource
    required: true
`
	// Modern layout with a bare (non-org-qualified) type.
	specResourcesBlockBareType = `
resources:
  your_first_artifact:
    resource_type: getting-started-resource
    required: true
`
	// Legacy layout: artifacts.properties.<field>.$ref.
	specArtifactsBlock = `
artifacts:
  required:
    - your_first_artifact
  properties:
    your_first_artifact:
      $ref: getting-started-resource
`
	// Both present — `resources` must win.
	specBothBlocks = `
resources:
  your_first_artifact:
    resource_type: chrissbx/from-resources-block
    required: true
artifacts:
  required:
    - your_first_artifact
  properties:
    your_first_artifact:
      $ref: from-artifacts-block
`
)

// resolveResourceType must read the modern `resources` block while still
// honoring the legacy `artifacts` block, so bundles written against either
// layout keep deploying on the v1 provider.
func TestResolveResourceTypeSupportsBothSpecLayouts(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name string
		spec string
		want string
	}{
		{
			name: "resources block, org-qualified type passes through",
			spec: specResourcesBlock,
			want: "chrissbx/getting-started-resource",
		},
		{
			name: "resources block, bare type gets the org prefix",
			spec: specResourcesBlockBareType,
			want: testOrgID + "/getting-started-resource",
		},
		{
			name: "legacy artifacts block still works",
			spec: specArtifactsBlock,
			want: testOrgID + "/getting-started-resource",
		},
		{
			name: "resources block wins when both are present",
			spec: specBothBlocks,
			want: "chrissbx/from-resources-block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
				"field":              "your_first_artifact",
				"name":               "My Resource",
				"resource":           `{}`,
				"specification_path": writeSpec(t, tt.spec),
			})

			got, err := resolveResourceType(rd, pc.Client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveResourceTypeErrors(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name    string
		spec    string
		wantMsg string
	}{
		{
			name:    "field in neither block",
			spec:    "resources:\n  some_other_field:\n    resource_type: a/b\n",
			wantMsg: `not found in "resources" or "artifacts"`,
		},
		{
			name:    "resources entry with empty resource_type",
			spec:    "resources:\n  your_first_artifact:\n    required: true\n",
			wantMsg: "has empty resource_type",
		},
		{
			name:    "artifacts entry with no $ref",
			spec:    "artifacts:\n  properties:\n    your_first_artifact:\n      description: nope\n",
			wantMsg: "has no $ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
				"field":              "your_first_artifact",
				"name":               "My Resource",
				"resource":           `{}`,
				"specification_path": writeSpec(t, tt.spec),
			})

			_, err := resolveResourceType(rd, pc.Client)
			if err == nil {
				t.Fatal("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("got error %q, want one containing %q", err, tt.wantMsg)
			}
		})
	}
}

// massdriver_artifact is what most v1 bundles actually use, so it needs the
// same forward compatibility as massdriver_resource.
func TestGetArtifactTypeSupportsBothSpecLayouts(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name string
		spec string
		want string
	}{
		{
			name: "resources block, org-qualified type passes through",
			spec: specResourcesBlock,
			want: "chrissbx/getting-started-resource",
		},
		{
			name: "resources block, bare type gets the org prefix",
			spec: specResourcesBlockBareType,
			want: testOrgID + "/getting-started-resource",
		},
		{
			name: "legacy artifacts block still works",
			spec: specArtifactsBlock,
			want: testOrgID + "/getting-started-resource",
		},
		{
			name: "resources block wins when both are present",
			spec: specBothBlocks,
			want: "chrissbx/from-resources-block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, resourceArtifact().Schema, map[string]any{
				"field":              "your_first_artifact",
				"name":               "My Artifact",
				"artifact":           `{}`,
				"specification_path": writeSpec(t, tt.spec),
			})

			got, err := getArtifactType(rd, pc.Client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
