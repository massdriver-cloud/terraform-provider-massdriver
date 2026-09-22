package massdriver

import (
	"context"
	"fmt"
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

	createInput *provresources.ResourceInput
	getID       string
	updateID    string
	updateInput *provresources.ResourceInput
	deleteID    string

	createCalls, getCalls, updateCalls, deleteCalls int
}

func (f *fakeProvisioningResources) CreateResource(_ context.Context, input *provresources.ResourceInput) (*provresources.Resource, error) {
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeProvisioningResources) GetResource(_ context.Context, id string) (*provresources.Resource, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeProvisioningResources) UpdateResource(_ context.Context, id string, input *provresources.ResourceInput) (*provresources.Resource, error) {
	f.updateID = id
	f.updateInput = input
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

func TestResourceResourceCreate(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{ID: "res-1"},
		getResp: &provresources.Resource{
			ID:           "res-1",
			Field:        "vpc",
			Name:         "My VPC",
			ResourceType: "aws-vpc@1.2.3",
		},
	}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `{"arn":"arn:aws:ec2:us-east-1:111:vpc/vpc-abc"}`,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "res-1" {
		t.Errorf("got id %q, want res-1", rd.Id())
	}
	// Create hands off to Read, so the type comes from getResp.
	if got := rd.Get("resource_type").(string); got != "aws-vpc@1.2.3" {
		t.Errorf("got resource_type %q, want aws-vpc@1.2.3", got)
	}
	if fake.createCalls != 1 {
		t.Fatalf("CreateResource called %d times, want 1", fake.createCalls)
	}
	in := fake.createInput
	if in.Field != "vpc" || in.Name != "My VPC" {
		t.Errorf("got create input %+v", in)
	}
	if in.Payload["arn"] != "arn:aws:ec2:us-east-1:111:vpc/vpc-abc" {
		t.Errorf("got payload.arn %v", in.Payload["arn"])
	}
}

// The server resolves the type, so no type is sent on the wire.
func TestResourceResourceCreateSendsNoType(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{ID: "res-1"},
		getResp:    &provresources.Resource{ID: "res-1", Field: "vpc"},
	}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `{"k":"v"}`,
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// ResourceInput has no type field, so assert the shape that is sent.
	in := fake.createInput
	if in.Field != "vpc" || in.Name != "My VPC" || len(in.Payload) != 1 {
		t.Errorf("got create input %+v, want only field/name/payload populated", in)
	}
}

func TestResourceResourceCreateRejectsInvalidJSON(t *testing.T) {
	fake := &fakeProvisioningResources{}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `not json`,
	})

	diags := resourceResourceCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "invalid JSON") {
		t.Errorf("error %q should name the bad JSON", diags[0].Summary)
	}
	if fake.createCalls != 0 {
		t.Errorf("expected 0 Create calls when the payload won't parse, got %d", fake.createCalls)
	}
}

func TestResourceResourceRead(t *testing.T) {
	fake := &fakeProvisioningResources{
		getResp: &provresources.Resource{
			ID:               "res-1",
			Field:            "vpc",
			Name:             "Server-side Name",
			Type:             testOrgID + "/aws-vpc",
			ResourceType:     "aws-vpc@1.2.3",
			AvailableUpgrade: "1.3.0",
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
	// The versioned `resource_type`, not the legacy org-qualified `type`.
	if rd.Get("resource_type").(string) != "aws-vpc@1.2.3" {
		t.Errorf("got resource_type %q, want aws-vpc@1.2.3", rd.Get("resource_type"))
	}
	if rd.Get("available_upgrade").(string) != "1.3.0" {
		t.Errorf("got available_upgrade %q, want 1.3.0", rd.Get("available_upgrade"))
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
		ID:           "res-1",
		Field:        "vpc",
		Name:         "Updated",
		ResourceType: "aws-vpc@1.2.4",
	}
	fake := &fakeProvisioningResources{updateResp: updated, getResp: updated}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":    "vpc",
		"name":     "Updated",
		"resource": `{"arn":"new"}`,
	})
	rd.SetId("res-1")

	if diags := resourceResourceUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.updateID != "res-1" {
		t.Errorf("got updateID %q, want res-1", fake.updateID)
	}
	if fake.updateInput.Name != "Updated" || fake.updateInput.Field != "vpc" {
		t.Errorf("got input %+v", fake.updateInput)
	}
	if rd.Get("resource_type").(string) != "aws-vpc@1.2.4" {
		t.Errorf("got resource_type %q, want aws-vpc@1.2.4", rd.Get("resource_type"))
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

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `{"k":"v"}`,
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
	if res := r.Schema["resource"]; res == nil || !res.Required || !res.Sensitive {
		t.Error("resource should be Required+Sensitive")
	}
	if rt := r.Schema["resource_type"]; rt == nil || rt.Required || rt.Optional || !rt.Computed || rt.ForceNew {
		t.Error("resource_type should be Computed and NOT ForceNew (a version bump updates in place)")
	}
	// Kept as a deprecated no-op so configs carrying it keep working; removing
	// it outright would be a breaking change.
	sp := r.Schema["specification_path"]
	if sp == nil || !sp.Optional || !sp.Computed || sp.Deprecated == "" {
		t.Error("specification_path should be Optional+Computed and deprecated, not removed")
	}
	if au := r.Schema["available_upgrade"]; au == nil || au.Required || au.Optional || !au.Computed || au.ForceNew {
		t.Error("available_upgrade should be Computed and NOT ForceNew")
	}
	if r.CustomizeDiff == nil {
		t.Error("CustomizeDiff should be set (it turns a pending upgrade into an in-place update)")
	}
	// `field` is the only attribute that may force replacement.
	for name, s := range r.Schema {
		if s.ForceNew && name != "field" {
			t.Errorf("%s is ForceNew; only `field` should replace the resource", name)
		}
	}
}

// resourceStateAndConfig builds a prior state holding the given resolved
// type, plus a config matching it on every user-settable attribute.
func resourceStateAndConfig(stateType, name, availableUpgrade string) (*terraform.InstanceState, *terraform.ResourceConfig) {
	state := &terraform.InstanceState{
		ID: "res-1",
		Attributes: map[string]string{
			"id":                "res-1",
			"field":             "vpc",
			"name":              "My VPC",
			"resource":          `{"k":"v"}`,
			"resource_type":     stateType,
			"available_upgrade": availableUpgrade,
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"field":    "vpc",
		"name":     name,
		"resource": `{"k":"v"}`,
	})
	return state, cfg
}

func TestResourceResourceVersionChangeNeverReplaces(t *testing.T) {
	state, cfg := resourceStateAndConfig("aws-vpc@1.2.3", "Renamed", "")

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil {
		t.Fatal("expected a diff for the renamed resource, got none")
	}
	if diff.RequiresNew() {
		t.Error("a name change must update in place, not replace")
	}
}

// No perpetual diff from the computed attributes.
func TestResourceResourceNoDiffWhenUnchanged(t *testing.T) {
	state, cfg := resourceStateAndConfig("aws-vpc@1.2.3", "My VPC", "")

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Fatalf("expected no diff, got %+v", diff)
	}
}

// `field` is the one attribute that still replaces.
func TestResourceResourceFieldChangeReplaces(t *testing.T) {
	state, _ := resourceStateAndConfig("aws-vpc@1.2.3", "My VPC", "")
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"field":    "network",
		"name":     "My VPC",
		"resource": `{"k":"v"}`,
	})

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || !diff.RequiresNew() {
		t.Error("a field change should force replacement")
	}
}

func TestResourceResourceAvailableUpgradePlansInPlaceUpdate(t *testing.T) {
	state, cfg := resourceStateAndConfig("aws-vpc@1.2.3", "My VPC", "1.3.0")

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("a pending upgrade should plan a diff, got none")
	}
	if diff.RequiresNew() {
		t.Error("an upgrade must update in place, never replace")
	}
	attr := diff.Attributes["resource_type"]
	if attr == nil || attr.Old != "aws-vpc@1.2.3" || attr.New != "aws-vpc@1.3.0" || attr.NewComputed {
		t.Errorf("got resource_type diff %+v, want aws-vpc@1.2.3 -> aws-vpc@1.3.0", attr)
	}
	// helper/schema won't plan a computed attribute as empty.
	if attr := diff.Attributes["available_upgrade"]; attr == nil || !attr.NewComputed {
		t.Errorf("got available_upgrade diff %+v, want it planned as unknown", attr)
	}
}

// This case must stay quiet, or every deploy writes every resource.
func TestResourceResourceNoUpgradePlansNothing(t *testing.T) {
	state, cfg := resourceStateAndConfig("aws-vpc@1.3.0", "My VPC", "")

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Fatalf("expected no diff when no upgrade is available, got %+v", diff)
	}
}

// CustomizeDiff runs on create too, where there is no prior state.
func TestResourceResourceCustomizeDiffNoopOnCreate(t *testing.T) {
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `{"k":"v"}`,
	})

	diff, err := resourceResource().Diff(t.Context(), nil, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil {
		t.Fatal("expected a create diff")
	}
}

// specification_path is accepted and ignored: a config still setting it plans
// no change, and nothing about it reaches the API.
func TestResourceResourceSpecificationPathIgnored(t *testing.T) {
	fake := &fakeProvisioningResources{
		createResp: &provresources.Resource{ID: "res-1"},
		getResp:    &provresources.Resource{ID: "res-1", Field: "vpc", ResourceType: "aws-vpc@1.0.0"},
	}
	pc := providerForResource(fake)

	rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{
		"field":              "vpc",
		"name":               "My VPC",
		"resource":           `{"k":"v"}`,
		"specification_path": "/nonexistent/massdriver.yaml",
	})

	if diags := resourceResourceCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("a set specification_path must not affect create; got %v", diags)
	}
	if fake.createInput.Field != "vpc" || fake.createInput.Name != "My VPC" {
		t.Errorf("got create input %+v", fake.createInput)
	}
}

// State written by 2.2.x carries specification_path. Upgrading must not turn
// that into a diff.
func TestResourceResourceNoDiffFromInheritedSpecificationPath(t *testing.T) {
	state := &terraform.InstanceState{
		ID: "res-1",
		Attributes: map[string]string{
			"id":                 "res-1",
			"field":              "vpc",
			"name":               "My VPC",
			"resource":           `{"k":"v"}`,
			"resource_type":      "aws-vpc@1.2.3",
			"available_upgrade":  "",
			"specification_path": "../massdriver.yaml",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"field":    "vpc",
		"name":     "My VPC",
		"resource": `{"k":"v"}`,
	})

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Fatalf("inherited specification_path should not produce a diff, got %+v", diff)
	}
}

// An API older than this change returns the legacy `type` and no
// resource_type; Read falls back to it rather than blanking state.
func TestResourceResourceReadFallsBackToLegacyType(t *testing.T) {
	cases := []struct{ name, legacy, resourceType, want string }{
		{"old api, org-qualified", testOrgID + "/aws-vpc", "", "aws-vpc"},
		{"old api, bare", "aws-vpc", "", "aws-vpc"},
		{"current api wins over legacy", testOrgID + "/aws-vpc", "aws-vpc@1.2.3", "aws-vpc@1.2.3"},
		{"neither", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeProvisioningResources{
				getResp: &provresources.Resource{
					ID: "res-1", Field: "vpc", Name: "My VPC",
					Type: tc.legacy, ResourceType: tc.resourceType,
				},
			}
			rd := schema.TestResourceDataRaw(t, resourceResource().Schema, map[string]any{})
			rd.SetId("res-1")

			if diags := resourceResourceRead(t.Context(), rd, providerForResource(fake)); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := rd.Get("resource_type").(string); got != tc.want {
				t.Errorf("got resource_type %q, want %q", got, tc.want)
			}
		})
	}
}

// An old API returns no available_upgrade, so the upgrade path stays inert:
// no spurious update, and nothing is replaced.
func TestResourceResourceOldAPIPlansNothing(t *testing.T) {
	state, cfg := resourceStateAndConfig("aws-vpc", "My VPC", "")

	diff, err := resourceResource().Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Fatalf("expected no diff against an old API, got %+v", diff)
	}
}
