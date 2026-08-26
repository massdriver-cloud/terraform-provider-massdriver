package massdriver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
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
func (f *fakeProvisioningResources) DeleteResource(_ context.Context, id string) error {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteErr
}

// providerForResource builds a ProviderClient whose ProvisioningResources
// thunk returns the supplied fake.
func providerForResource(fake *fakeProvisioningResources) *ProviderClient {
	return &ProviderClient{
		Config: config.Config{OrganizationID: testOrgID},
		ProvisioningResources: func() (provisioningResourcesAPI, error) {
			return fake, nil
		},
	}
}

// writeSpec writes the given massdriver.yaml contents into a temp dir and
// returns the file's path.
func writeSpec(t *testing.T, yaml string) string {
	t.Helper()
	specPath := filepath.Join(t.TempDir(), "massdriver.yaml")
	if err := os.WriteFile(specPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return specPath
}

// resourcesSpec declares `field` under the `resources` block.
func resourcesSpec(field, resourceType string) string {
	return "resources:\n  " + field + ":\n    resource_type: " + resourceType + "\n    required: true\n"
}

// artifactsSpec declares `field` under the legacy `artifacts.properties`
// block with a $ref.
func artifactsSpec(field, ref string) string {
	return "artifacts:\n  properties:\n    " + field + ":\n      $ref: " + ref + "\n"
}

func TestResourceResourceCreate(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{
			ID:    "res-1",
			Field: "vpc",
			Name:  "My VPC",
			Type:  "aws-vpc",
		},
		getResp: &provresources.Resource{
			ID:    "res-1",
			Field: "vpc",
			Name:  "My VPC",
			Type:  "aws-vpc",
		},
	}
	pc := providerForResource(fake)

	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc"))
	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"arn":"arn:aws:ec2:us-east-1:111:vpc/vpc-abc"}`,
		"specification_path": specPath,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "res-1" {
		t.Errorf("got id %q, want res-1", rd.Id())
	}
	if got := rd.Get("resource_type").(string); got != "aws-vpc" {
		t.Errorf("got resource_type %q, want aws-vpc", got)
	}

	if fake.createCalls != 1 {
		t.Fatalf("CreateResource called %d times, want 1", fake.createCalls)
	}
	in := fake.createInput
	if in.Field != "vpc" || in.Name != "My VPC" {
		t.Errorf("got create input %+v", in)
	}
	if in.Type != "aws-vpc" {
		t.Errorf("got Type %q, want aws-vpc", in.Type)
	}
	if in.Payload["arn"] != "arn:aws:ec2:us-east-1:111:vpc/vpc-abc" {
		t.Errorf("got payload.arn %v", in.Payload["arn"])
	}
}

// The legacy `artifacts.properties.<field>.$ref` path still resolves the
// type, and the ref is sent verbatim — no org prefixing.
func TestResourceResourceCreateLegacyArtifactsRefSentVerbatim(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{ID: "res-1", Field: "vpc"},
		getResp:    &provresources.Resource{ID: "res-1", Field: "vpc"},
	}
	pc := providerForResource(fake)

	specPath := writeSpec(t, artifactsSpec("vpc", "aws-vpc"))
	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"k":"v"}`,
		"specification_path": specPath,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := fake.createInput.Type; got != "aws-vpc" {
		t.Errorf("got Type %q, want aws-vpc (sent verbatim)", got)
	}
}

// A field missing from both `resources` and `artifacts` surfaces a clear
// error naming the field, before any API call.
func TestResourceResourceCreateRejectsUnknownField(t *testing.T) {
	fake := &fakeProvisioningResources{}
	pc := providerForResource(fake)

	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc"))
	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "database",
		"name":               "DB",
		"resource":           `{}`,
		"specification_path": specPath,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "database") {
		t.Errorf("error %q should mention the unknown field name", diags[0].Summary)
	}
	if fake.createCalls != 0 {
		t.Errorf("expected 0 Create calls when type lookup fails, got %d", fake.createCalls)
	}
}

func TestResourceResourceRead(t *testing.T) {
	// The API echoes back an org-qualified type; Read must normalize it so the
	// next plan's comparison against the (bare) yaml value stays clean.
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
	if rd.Get("resource_type").(string) != "aws-vpc" {
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
		Type:  "aws-vpc",
	}
	fake := &fakeProvisioningResources{updateResp: updated, getResp: updated}
	pc := providerForResource(fake)

	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc"))
	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "Updated",
		"resource":           `{"arn":"new"}`,
		"specification_path": specPath,
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

	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc"))
	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"k":"v"}`,
		"specification_path": specPath,
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
	if sp := r.Schema["specification_path"]; sp.Default != defaultResourceSpecificationPath {
		t.Errorf("got specification_path default %v, want %s", sp.Default, defaultResourceSpecificationPath)
	}
	if r.CustomizeDiff == nil {
		t.Error("CustomizeDiff should be set (re-resolves resource_type from massdriver.yaml at plan time)")
	}
}

func TestResolveResourceType(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		field   string
		want    string
		wantErr string
	}{
		{"resources block", resourcesSpec("vpc", "aws-vpc@1.0.0"), "vpc", "aws-vpc@1.0.0", ""},
		{"resources wins over legacy artifacts", resourcesSpec("vpc", "aws-vpc@2.0.0") + artifactsSpec("vpc", "aws-vpc"), "vpc", "aws-vpc@2.0.0", ""},
		{"legacy artifacts $ref normalized to bare type", artifactsSpec("vpc", "other-org/aws-vpc"), "vpc", "aws-vpc", ""},
		{"qualified versioned type normalized", resourcesSpec("vpc", "some-org/aws-vpc@2.0.0"), "vpc", "aws-vpc@2.0.0", ""},
		{"empty resource_type", "resources:\n  vpc:\n    required: true\n", "vpc", "", "empty resource_type"},
		{"empty $ref", "artifacts:\n  properties:\n    vpc: {}\n", "vpc", "", "empty $ref"},
		{"unknown field", resourcesSpec("vpc", "aws-vpc"), "database", "", "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveResourceType(tc.field, writeSpec(t, tc.spec))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got err %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := resolveResourceType("vpc", filepath.Join(t.TempDir(), "nope.yaml")); err == nil || !strings.Contains(err.Error(), "unable to open") {
			t.Fatalf("got err %v, want unable-to-open error", err)
		}
	})
}

// resourceStateAndConfig builds a matching prior state and config for a
// resource whose last apply stored `stateType`, pointing at specPath.
func resourceStateAndConfig(stateType, specPath string) (*terraform.InstanceState, *terraform.ResourceConfig) {
	state := &terraform.InstanceState{
		ID: "res-1",
		Attributes: map[string]string{
			"id":                 "res-1",
			"field":              "vpc",
			"name":               "My VPC",
			"resource":           `{"k":"v"}`,
			"resource_type":      stateType,
			"specification_path": specPath,
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"k":"v"}`,
		"specification_path": specPath,
	})
	return state, cfg
}

// A resource_type change in massdriver.yaml alone — with no change to the
// terraform config — must plan a replacement: CustomizeDiff re-resolves the
// type at plan time and resource_type is ForceNew.
func TestResourceResourceTypeBumpPlansReplacement(t *testing.T) {
	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc@2.0.0"))
	state, cfg := resourceStateAndConfig("aws-vpc@1.0.0", specPath)

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil {
		t.Fatal("expected a diff, got none")
	}
	attr := diff.Attributes["resource_type"]
	if attr == nil || attr.New != "aws-vpc@2.0.0" {
		t.Fatalf("got resource_type diff %+v, want new value aws-vpc@2.0.0", attr)
	}
	if !diff.RequiresNew() {
		t.Error("resource_type change should force replacement")
	}
}

// With the yaml type matching state, an otherwise-unchanged resource plans no
// diff at all.
func TestResourceResourceNoDiffWhenTypeUnchanged(t *testing.T) {
	specPath := writeSpec(t, resourcesSpec("vpc", "aws-vpc@1.0.0"))
	state, cfg := resourceStateAndConfig("aws-vpc@1.0.0", specPath)

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Fatalf("expected no diff, got %+v", diff)
	}
}
