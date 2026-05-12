package massdriver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
)

// recordedRequest captures one HTTP exchange so tests can assert on what the
// SDK sent.
type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

// newRESTMockProvider stands up an httptest server with the given handler,
// builds a *ProviderClient pointed at it, and returns the captured requests
// slice plus a cleanup-registered server so callers don't need to defer Close.
func newRESTMockProvider(t *testing.T, handler http.HandlerFunc) (*ProviderClient, *[]recordedRequest) {
	t.Helper()
	requests := &[]recordedRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		*requests = append(*requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   parsed,
		})
		// resty's SetResult only decodes when the response declares JSON; set
		// it here once so each handler doesn't have to remember.
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	pc := &ProviderClient{
		Client: &client.Client{
			Config: config.Config{
				URL:            srv.URL,
				OrganizationID: testOrgID,
				// massdriver_resource fast-fails on non-deployment auth, so the
				// test client has to look like it ran inside a bundle deployment.
				Credentials: &config.Credentials{Method: config.AuthDeployment},
			},
			HTTP: resty.New().
				SetBaseURL(srv.URL).
				SetHeader("Content-Type", "application/json").
				SetHeader("Accept", "application/json"),
		},
	}
	return pc, requests
}

// writeBundleFiles writes minimal massdriver.yaml + schema-artifacts.json into
// a temp dir and returns the two paths. The schema file declares one field's
// JSON Schema; the spec file declares the same field's $ref so the type
// lookup succeeds.
func writeBundleFiles(t *testing.T, field, ref string, fieldSchema map[string]any) (specPath, schemaPath string) {
	t.Helper()
	dir := t.TempDir()

	specPath = filepath.Join(dir, "massdriver.yaml")
	specYAML := "artifacts:\n  properties:\n    " + field + ":\n      $ref: " + ref + "\n"
	if err := os.WriteFile(specPath, []byte(specYAML), 0644); err != nil {
		t.Fatal(err)
	}

	schemaPath = filepath.Join(dir, "schema-artifacts.json")
	schemaDoc := map[string]any{
		"properties": map[string]any{
			field: fieldSchema,
		},
	}
	schemaBytes, _ := json.Marshal(schemaDoc)
	if err := os.WriteFile(schemaPath, schemaBytes, 0644); err != nil {
		t.Fatal(err)
	}
	return specPath, schemaPath
}

// objectSchema is a permissive "anything goes as long as it's an object" JSON
// Schema — useful for tests that don't care about field-level validation.
func objectSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func TestResourceResourceCreate(t *testing.T) {
	pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "res-1",
			"field":   "vpc",
			"name":    "My VPC",
			"type":    testOrgID + "/aws-vpc",
			"payload": map[string]any{"arn": "arn:aws:ec2:us-east-1:111:vpc/vpc-abc"},
		})
	})
	specPath, schemaPath := writeBundleFiles(t, "vpc", "aws-vpc", objectSchema())

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"arn":"arn:aws:ec2:us-east-1:111:vpc/vpc-abc"}`,
		"specification_path": specPath,
		"schema_path":        schemaPath,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "res-1" {
		t.Errorf("got id %q, want res-1", rd.Id())
	}
	// resource_type was looked up from massdriver.yaml's $ref and prefixed with the org ID.
	if got := rd.Get("resource_type").(string); got != testOrgID+"/aws-vpc" {
		t.Errorf("got resource_type %q, want %s/aws-vpc", got, testOrgID)
	}

	// Two HTTP calls: POST (create) then GET (post-create read).
	if len(*reqs) != 2 {
		t.Fatalf("got %d HTTP requests, want 2 (POST + GET)", len(*reqs))
	}
	post := (*reqs)[0]
	if post.Method != http.MethodPost || post.Path != "/v1/resources" {
		t.Errorf("got %s %s, want POST /v1/resources", post.Method, post.Path)
	}
	if post.Body["field"] != "vpc" {
		t.Errorf("got body.field %v, want vpc", post.Body["field"])
	}
	if post.Body["name"] != "My VPC" {
		t.Errorf("got body.name %v, want My VPC", post.Body["name"])
	}
	if post.Body["type"] != testOrgID+"/aws-vpc" {
		t.Errorf("got body.type %v, want %s/aws-vpc", post.Body["type"], testOrgID)
	}
	payload, ok := post.Body["payload"].(map[string]any)
	if !ok {
		t.Fatalf("body.payload should be a JSON object, got %T", post.Body["payload"])
	}
	if payload["arn"] != "arn:aws:ec2:us-east-1:111:vpc/vpc-abc" {
		t.Errorf("got payload.arn %v", payload["arn"])
	}
}

// A fully-qualified `$ref` (already containing a slash) is sent as-is — no
// double-prefixing with the org ID.
func TestResourceResourceCreateFullyQualifiedRefPassesThrough(t *testing.T) {
	pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "res-1", "field": "vpc"})
	})
	specPath, schemaPath := writeBundleFiles(t, "vpc", "other-org/aws-vpc", objectSchema())

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"k":"v"}`,
		"specification_path": specPath,
		"schema_path":        schemaPath,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := (*reqs)[0].Body["type"]; got != "other-org/aws-vpc" {
		t.Errorf("got body.type %v, want other-org/aws-vpc (no double-prefix)", got)
	}
}

// Schema validation runs *before* the API call — bad payloads must not reach
// the server.
func TestResourceResourceCreateRejectsInvalidPayloadAgainstSchema(t *testing.T) {
	pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when client-side validation fails")
		w.WriteHeader(http.StatusInternalServerError)
	})
	// Schema requires `arn` as a required string property; the user's payload omits it.
	fieldSchema := map[string]any{
		"type":     "object",
		"required": []any{"arn"},
		"properties": map[string]any{
			"arn": map[string]any{"type": "string"},
		},
	}
	specPath, schemaPath := writeBundleFiles(t, "vpc", "aws-vpc", fieldSchema)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"not_arn":"oops"}`,
		"specification_path": specPath,
		"schema_path":        schemaPath,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected validation error, got none")
	}
	if !strings.Contains(diags[0].Summary, "validation failed") {
		t.Errorf("got error %q, want one mentioning validation failure", diags[0].Summary)
	}
	if len(*reqs) != 0 {
		t.Errorf("expected 0 HTTP calls when validation fails, got %d", len(*reqs))
	}
}

// A field that exists in massdriver.yaml but not in schema-artifacts.json
// surfaces a clear error rather than silently skipping validation.
func TestResourceResourceCreateRejectsUnknownField(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {})
	// schema only declares "vpc", but the user's resource references "database".
	specPath, schemaPath := writeBundleFiles(t, "vpc", "aws-vpc", objectSchema())

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "database",
		"name":               "DB",
		"resource":           `{}`,
		"specification_path": specPath,
		"schema_path":        schemaPath,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "database") {
		t.Errorf("error %q should mention the unknown field name", diags[0].Summary)
	}
}

func TestResourceResourceRead(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "res-1",
			"field": "vpc",
			"name":  "Server-side Name",
			"type":  testOrgID + "/aws-vpc",
		})
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	if diags := resourceResourceRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "Server-side Name" {
		t.Errorf("got name %q", rd.Get("name"))
	}
	if rd.Get("field").(string) != "vpc" {
		t.Errorf("got field %q, want vpc", rd.Get("field"))
	}
	if rd.Get("resource_type").(string) != testOrgID+"/aws-vpc" {
		t.Errorf("got resource_type %q", rd.Get("resource_type"))
	}
}

// A 404 from the REST API means the resource was deleted out of band — Read
// must clear state so terraform plans a re-create.
func TestResourceResourceReadClearsOn404(t *testing.T) {
	pc, _ := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	if diags := resourceResourceRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on 404, got %q", rd.Id())
	}
}

func TestResourceResourceUpdate(t *testing.T) {
	pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "res-1",
			"field": "vpc",
			"name":  "Updated",
			"type":  testOrgID + "/aws-vpc",
		})
	})
	specPath, schemaPath := writeBundleFiles(t, "vpc", "aws-vpc", objectSchema())

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "Updated",
		"resource":           `{"arn":"new"}`,
		"specification_path": specPath,
		"schema_path":        schemaPath,
	})
	rd.SetId("res-1")

	if diags := resourceResourceUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	put := (*reqs)[0]
	if put.Method != http.MethodPut || put.Path != "/v1/resources/res-1" {
		t.Errorf("got %s %s, want PUT /v1/resources/res-1", put.Method, put.Path)
	}
	if put.Body["name"] != "Updated" {
		t.Errorf("got body.name %v, want Updated", put.Body["name"])
	}
}

func TestResourceResourceDelete(t *testing.T) {
	pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field": "vpc",
	})
	rd.SetId("res-1")

	if diags := resourceResourceDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared after delete, got %q", rd.Id())
	}

	del := (*reqs)[0]
	if del.Method != http.MethodDelete || del.Path != "/v1/resources/res-1" {
		t.Errorf("got %s %s, want DELETE /v1/resources/res-1", del.Method, del.Path)
	}
	// The SDK sends the field in the request body so the server can match against the right artifact slot.
	if del.Body["field"] != "vpc" {
		t.Errorf("got delete body.field %v, want vpc", del.Body["field"])
	}
}

// Non-deployment auth (api_key, PAT, or no credentials at all) must fail
// before any HTTP call. The REST endpoint rejects those auth methods anyway,
// but the local check produces a clearer error and avoids round-tripping a
// 401.
func TestResourceResourceRejectsNonDeploymentAuth(t *testing.T) {
	cases := []struct {
		name        string
		credentials *config.Credentials
	}{
		{name: "api_key", credentials: &config.Credentials{Method: config.AuthAPIKey}},
		{name: "personal_access_token", credentials: &config.Credentials{Method: config.AuthPAT}},
		{name: "no_credentials", credentials: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pc, reqs := newRESTMockProvider(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("HTTP server should not be called when auth check fails")
			})
			pc.Client.Config.Credentials = tc.credentials // override deployment-auth default

			specPath, schemaPath := writeBundleFiles(t, "vpc", "aws-vpc", objectSchema())

			rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
				"field":              "vpc",
				"name":               "My VPC",
				"resource":           `{"k":"v"}`,
				"specification_path": specPath,
				"schema_path":        schemaPath,
			})

			diags := resourceResourceCreate(t.Context(), rd, pc)
			if !diags.HasError() {
				t.Fatal("expected auth-method error, got none")
			}
			if !strings.Contains(diags[0].Summary, "deployment") {
				t.Errorf("error %q should mention deployment auth requirement", diags[0].Summary)
			}
			if rd.Id() != "" {
				t.Errorf("ID should not be set when auth check fails, got %q", rd.Id())
			}
			if len(*reqs) != 0 {
				t.Errorf("expected 0 HTTP calls, got %d", len(*reqs))
			}
		})
	}
}

func TestResourceResourceSchema(t *testing.T) {
	r := resourceResource()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if f := r.Schema["field"]; f == nil || !f.Required || !f.ForceNew {
		t.Error("field should be Required+ForceNew (the SDK delete path needs it stable)")
	}
	if rt := r.Schema["resource_type"]; rt == nil || rt.Required || rt.Optional || !rt.Computed || !rt.ForceNew {
		t.Error("resource_type should be Computed+ForceNew (derived from massdriver.yaml, not user-supplied)")
	}
	if res := r.Schema["resource"]; res == nil || !res.Required || !res.Sensitive {
		t.Error("resource should be Required+Sensitive")
	}
	// schema_path / specification_path default to the bundle scaffolding's standard locations.
	if sp := r.Schema["schema_path"]; sp.Default != defaultResourceSchemaPath {
		t.Errorf("got schema_path default %v, want %s", sp.Default, defaultResourceSchemaPath)
	}
	if sp := r.Schema["specification_path"]; sp.Default != defaultResourceSpecificationPath {
		t.Errorf("got specification_path default %v, want %s", sp.Default, defaultResourceSpecificationPath)
	}
}
