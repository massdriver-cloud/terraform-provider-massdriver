package massdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	provresources "github.com/massdriver-cloud/massdriver-sdk-go/massdriver/provisioning/resources"
)

// fakeProvisioningResources records every call for assertion and returns
// whatever canned response the test wires in. Satisfies
// provisioningResourcesAPI.
type fakeProvisioningResources struct {
	createResp, getResp, updateResp *provresources.Resource
	createErr, getErr, updateErr    error
	deleteErr                       error

	createInput *provresources.Resource
	getID       string
	updateID    string
	updateInput *provresources.Resource
	deleteID    string
	deleteField string

	createCalls, getCalls, updateCalls, deleteCalls int
}

func (f *fakeProvisioningResources) CreateResource(_ context.Context, a *provresources.Resource) (*provresources.Resource, error) {
	f.createInput = a
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeProvisioningResources) GetResource(_ context.Context, id string) (*provresources.Resource, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeProvisioningResources) UpdateResource(_ context.Context, id string, a *provresources.Resource) (*provresources.Resource, error) {
	f.updateID = id
	f.updateInput = a
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeProvisioningResources) DeleteResource(_ context.Context, id, field string) error {
	f.deleteID = id
	f.deleteField = field
	f.deleteCalls++
	return f.deleteErr
}

// providerForResource builds a ProviderClient whose ProvisioningResources
// thunk returns the supplied fake. Config carries the testOrgID so
// resolveResourceType can prefix bare type refs.
func providerForResource(fake *fakeProvisioningResources) *ProviderClient {
	return &ProviderClient{
		Config: config.Config{OrganizationID: testOrgID},
		ProvisioningResources: func() (provisioningResourcesAPI, error) {
			return fake, nil
		},
	}
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
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{
			ID:    "res-1",
			Field: "vpc",
			Name:  "My VPC",
			Type:  testOrgID + "/aws-vpc",
		},
		getResp: &provresources.Resource{
			ID:    "res-1",
			Field: "vpc",
			Name:  "My VPC",
			Type:  testOrgID + "/aws-vpc",
		},
	}
	pc := providerForResource(fake)

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
	if got := rd.Get("resource_type").(string); got != testOrgID+"/aws-vpc" {
		t.Errorf("got resource_type %q, want %s/aws-vpc", got, testOrgID)
	}

	if fake.createCalls != 1 {
		t.Fatalf("CreateResource called %d times, want 1", fake.createCalls)
	}
	in := fake.createInput
	if in.Field != "vpc" || in.Name != "My VPC" {
		t.Errorf("got create input %+v", in)
	}
	if in.Type != testOrgID+"/aws-vpc" {
		t.Errorf("got Type %q, want %s/aws-vpc", in.Type, testOrgID)
	}
	if in.Payload["arn"] != "arn:aws:ec2:us-east-1:111:vpc/vpc-abc" {
		t.Errorf("got payload.arn %v", in.Payload["arn"])
	}
}

// A fully-qualified `$ref` (already containing a slash) is sent as-is — no
// double-prefixing with the org ID.
func TestResourceResourceCreateFullyQualifiedRefPassesThrough(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{ID: "res-1", Field: "vpc"},
		getResp:    &provresources.Resource{ID: "res-1", Field: "vpc"},
	}
	pc := providerForResource(fake)

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
	if got := fake.createInput.Type; got != "other-org/aws-vpc" {
		t.Errorf("got Type %q, want other-org/aws-vpc (no double-prefix)", got)
	}
}

// Schema validation runs *before* the API call — bad payloads must not reach
// the SDK.
func TestResourceResourceCreateRejectsInvalidPayloadAgainstSchema(t *testing.T) {
	fake := &fakeProvisioningResources{}
	pc := providerForResource(fake)

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
	if fake.createCalls != 0 {
		t.Errorf("expected 0 Create calls when validation fails, got %d", fake.createCalls)
	}
}

// A field that exists in massdriver.yaml but not in schema-artifacts.json
// surfaces a clear error rather than silently skipping validation.
func TestResourceResourceCreateRejectsUnknownField(t *testing.T) {
	fake := &fakeProvisioningResources{}
	pc := providerForResource(fake)

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
	fake := &fakeProvisioningResources{
		getResp: &provresources.Resource{
			ID:    "res-1",
			Field: "vpc",
			Name:  "Server-side Name",
			Type:  testOrgID + "/aws-vpc",
		},
	}
	pc := providerForResource(fake)

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

// The provisioning REST surface wraps 404s with provresources.ErrNotFound.
// Read must detect it and clear state so terraform plans a recreate.
func TestResourceResourceReadClearsOnNotFound(t *testing.T) {
	fake := &fakeProvisioningResources{getErr: fmt.Errorf("get resource res-1: %w", provresources.ErrNotFound)}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
	rd.SetId("res-1")

	if diags := resourceResourceRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on 404, got %q", rd.Id())
	}
}

func TestResourceResourceUpdate(t *testing.T) {
	updated := &provresources.Resource{
		ID:    "res-1",
		Field: "vpc",
		Name:  "Updated",
		Type:  testOrgID + "/aws-vpc",
	}
	fake := &fakeProvisioningResources{updateResp: updated, getResp: updated}
	pc := providerForResource(fake)

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

	if fake.updateID != "res-1" {
		t.Errorf("got updateID %q, want res-1", fake.updateID)
	}
	if fake.updateInput.Name != "Updated" {
		t.Errorf("got input.Name %q, want Updated", fake.updateInput.Name)
	}
}

func TestResourceResourceDelete(t *testing.T) {
	fake := &fakeProvisioningResources{}
	pc := providerForResource(fake)

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
	if fake.deleteID != "res-1" {
		t.Errorf("got deleteID %q, want res-1", fake.deleteID)
	}
	if fake.deleteField != "vpc" {
		t.Errorf("got deleteField %q, want vpc (the SDK sends it in the body to match the right artifact slot)", fake.deleteField)
	}
}

// Delete returning ErrNotFound means the record is already gone — fine for
// destroy; we shouldn't error.
func TestResourceResourceDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeProvisioningResources{deleteErr: fmt.Errorf("delete resource gone: %w", provresources.ErrNotFound)}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field": "vpc",
	})
	rd.SetId("already-gone")

	if diags := resourceResourceDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

// The provisioning thunk errors when MASSDRIVER_DEPLOYMENT_ID / MASSDRIVER_TOKEN
// aren't set — i.e. when the provider runs outside a bundle deployment. CRUD
// must surface that error verbatim so the user sees the missing env vars
// rather than an opaque 401 from the server.
func TestResourceResourceRejectsNonDeploymentAuth(t *testing.T) {
	authErr := fmt.Errorf("massdriver_resource can only be used inside a Massdriver bundle deployment (MASSDRIVER_DEPLOYMENT_ID + MASSDRIVER_TOKEN must be set)")
	pc := &ProviderClient{
		Config: config.Config{OrganizationID: testOrgID},
		ProvisioningResources: func() (provisioningResourcesAPI, error) {
			return nil, authErr
		},
	}

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
	if !strings.Contains(diags[0].Summary, "bundle deployment") {
		t.Errorf("error %q should mention bundle deployment requirement", diags[0].Summary)
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set when auth check fails, got %q", rd.Id())
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
	if sp := r.Schema["schema_path"]; sp.Default != defaultResourceSchemaPath {
		t.Errorf("got schema_path default %v, want %s", sp.Default, defaultResourceSchemaPath)
	}
	if sp := r.Schema["specification_path"]; sp.Default != defaultResourceSpecificationPath {
		t.Errorf("got specification_path default %v, want %s", sp.Default, defaultResourceSpecificationPath)
	}
}
