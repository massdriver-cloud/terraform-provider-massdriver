package massdriver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// The Massdriver CLI no longer emits schema-artifacts.json. `massdriver_artifact`
// is what most v1 bundles actually use, and it validated on both create and
// update — so an unavailable schema must skip validation rather than fail the
// apply and force a bundle republish.
func TestValidateArtifactSkipsWhenSchemaUnavailable(t *testing.T) {
	dir := t.TempDir()

	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(dir, t.Name()+".json")
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	tests := []struct {
		name       string
		schemaPath func(t *testing.T) string
	}{
		{
			name: "file does not exist",
			schemaPath: func(t *testing.T) string {
				return filepath.Join(dir, "definitely-not-here.json")
			},
		},
		{
			name:       "file is not valid JSON",
			schemaPath: func(t *testing.T) string { return write(t, "not json at all") },
		},
		{
			name: "field has no schema",
			schemaPath: func(t *testing.T) string {
				return write(t, `{"properties":{"other":{"type":"object"}}}`)
			},
		},
		{
			name: "field schema is a boolean, not an object",
			schemaPath: func(t *testing.T) string {
				return write(t, `{"properties":{"vpc":true}}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, resourceArtifact().Schema, map[string]any{
				"field":       "vpc",
				"name":        "My VPC",
				"artifact":    `{"totally":"unvalidated"}`,
				"schema_path": tt.schemaPath(t),
			})
			if err := validateArtifact(rd); err != nil {
				t.Errorf("expected validation to be skipped, got error: %v", err)
			}
		})
	}
}

// A schema that IS present and resolvable is still enforced.
func TestValidateArtifactStillEnforcesAvailableSchema(t *testing.T) {
	p := filepath.Join(t.TempDir(), "schema-artifacts.json")
	doc := `{"properties":{"vpc":{"type":"object","required":["arn"],` +
		`"properties":{"arn":{"type":"string"}}}}}`
	if err := os.WriteFile(p, []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}

	rd := schema.TestResourceDataRaw(t, resourceArtifact().Schema, map[string]any{
		"field":       "vpc",
		"name":        "My VPC",
		"artifact":    `{"not_arn":"oops"}`,
		"schema_path": p,
	})
	if err := validateArtifact(rd); err == nil {
		t.Fatal("expected validation error when the schema is available and the artifact violates it")
	}
}
