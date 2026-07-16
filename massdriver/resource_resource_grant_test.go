package massdriver

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/resources"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakeResourceGrants struct {
	createResp *resources.Grant
	createErr  error
	deleteErr  error

	// grants is what IterGrants yields; iterErr, when set, is yielded as the
	// iterator's terminal error (how the SDK reports a missing parent).
	grants  []resources.Grant
	iterErr error

	createResourceID string
	createInput      resources.CreateGrantInput
	iterResourceID   string
	deleteID         string

	createCalls, iterCalls, deleteCalls int
}

func (f *fakeResourceGrants) CreateGrant(_ context.Context, resourceID string, input resources.CreateGrantInput) (*resources.Grant, error) {
	f.createResourceID = resourceID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}

func (f *fakeResourceGrants) DeleteGrant(_ context.Context, grantID string) error {
	f.deleteID = grantID
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeResourceGrants) IterGrants(_ context.Context, resourceID string, _ resources.ListGrantsInput) iter.Seq2[resources.Grant, error] {
	f.iterResourceID = resourceID
	f.iterCalls++
	return func(yield func(resources.Grant, error) bool) {
		if f.iterErr != nil {
			yield(resources.Grant{}, f.iterErr)
			return
		}
		for _, g := range f.grants {
			if !yield(g, nil) {
				return
			}
		}
	}
}

func TestResourceResourceGrantCreate(t *testing.T) {
	grant := resources.Grant{
		ID:                  "grant-1",
		Action:              "resource:export",
		RecipientConditions: types.PolicyConditions{"team": {"eng"}},
	}
	fake := &fakeResourceGrants{createResp: &grant, grants: []resources.Grant{grant}}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id":          "resource-1",
		"recipient_conditions": `{"team":["eng"]}`,
	})

	if diags := resourceResourceGrantCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "grant-1" {
		t.Errorf("got id %q, want grant-1", rd.Id())
	}
	if fake.createResourceID != "resource-1" {
		t.Errorf("got CreateGrant resourceID %q, want resource-1", fake.createResourceID)
	}
	in := fake.createInput
	if in.Action != "resource:export" {
		t.Errorf("got Action %q, want the hardcoded resource:export", in.Action)
	}
	if got := in.RecipientConditions["team"]; len(got) != 1 || got[0] != "eng" {
		t.Errorf("got RecipientConditions[team] %v, want [eng]", got)
	}
}

// `"*"` is the whole-grant wildcard — the SDK takes a nil map for that.
func TestResourceResourceGrantCreateWildcardConditions(t *testing.T) {
	grant := resources.Grant{ID: "grant-2", Action: "resource:export", RecipientConditions: nil}
	fake := &fakeResourceGrants{createResp: &grant, grants: []resources.Grant{grant}}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id":          "resource-1",
		"recipient_conditions": "*",
	})

	if diags := resourceResourceGrantCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.createInput.RecipientConditions != nil {
		t.Errorf("got RecipientConditions %v, want nil (wildcard sentinel)", fake.createInput.RecipientConditions)
	}
}

// Invalid JSON in recipient_conditions must error before the API call.
func TestResourceResourceGrantCreateRejectsInvalidConditionsJSON(t *testing.T) {
	fake := &fakeResourceGrants{}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id":          "resource-1",
		"recipient_conditions": `not json`,
	})

	diags := resourceResourceGrantCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected JSON parse error, got none")
	}
	if fake.createCalls != 0 {
		t.Errorf("CreateGrant should not fire on parse error; got %d calls", fake.createCalls)
	}
}

func TestResourceResourceGrantCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeResourceGrants{createErr: fmt.Errorf("create grant on resource resource-1: forbidden")}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id":          "resource-1",
		"recipient_conditions": "*",
	})

	diags := resourceResourceGrantCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "forbidden") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
}

func TestResourceResourceGrantRead(t *testing.T) {
	fake := &fakeResourceGrants{grants: []resources.Grant{
		{ID: "grant-other", Action: "resource:export", RecipientConditions: nil},
		{ID: "grant-1", Action: "resource:export", RecipientConditions: types.PolicyConditions{"team": {"eng"}}},
	}}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id": "resource-1",
	})
	rd.SetId("grant-1")

	if diags := resourceResourceGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.iterResourceID != "resource-1" {
		t.Errorf("got IterGrants resourceID %q, want resource-1", fake.iterResourceID)
	}
	if got := rd.Get("recipient_conditions").(string); got != `{"team":["eng"]}` {
		t.Errorf("got recipient_conditions %q, want plain JSON object", got)
	}
}

// A wildcard grant round-trips back to the literal "*" so the user's HCL
// doesn't manufacture drift on the next plan.
func TestResourceResourceGrantReadEncodesWildcard(t *testing.T) {
	fake := &fakeResourceGrants{grants: []resources.Grant{
		{ID: "grant-w", Action: "resource:export", RecipientConditions: nil},
	}}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id": "resource-1",
	})
	rd.SetId("grant-w")

	if diags := resourceResourceGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := rd.Get("recipient_conditions").(string); got != "*" {
		t.Errorf("nil conditions should encode to `*`; got %q", got)
	}
}

// The grant no longer appearing in the parent's list means it was deleted
// out-of-band — clear state so terraform plans a re-create.
func TestResourceResourceGrantReadClearsWhenGrantMissing(t *testing.T) {
	fake := &fakeResourceGrants{grants: []resources.Grant{{ID: "grant-other"}}}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id": "resource-1",
	})
	rd.SetId("gone")

	if diags := resourceResourceGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("missing grant should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared when grant is absent; got %q", rd.Id())
	}
}

// The parent resource being gone yields ErrNotFound from the iterator —
// treated the same as the grant itself being gone.
func TestResourceResourceGrantReadClearsWhenParentNotFound(t *testing.T) {
	fake := &fakeResourceGrants{iterErr: fmt.Errorf("list resource resource-1 grants: %w", gql.ErrNotFound)}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id": "resource-1",
	})
	rd.SetId("grant-1")

	if diags := resourceResourceGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("parent not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on parent not-found; got %q", rd.Id())
	}
}

func TestResourceResourceGrantReadPropagatesListFailure(t *testing.T) {
	fake := &fakeResourceGrants{iterErr: fmt.Errorf("list resource resource-1 grants: boom")}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{
		"resource_id": "resource-1",
	})
	rd.SetId("grant-1")

	diags := resourceResourceGrantRead(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "grant-1" {
		t.Errorf("ID should be preserved on transient failure; got %q", rd.Id())
	}
}

func TestResourceResourceGrantDelete(t *testing.T) {
	fake := &fakeResourceGrants{}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{})
	rd.SetId("grant-1")

	if diags := resourceResourceGrantDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "grant-1" {
		t.Errorf("got deleteID %q, want grant-1", fake.deleteID)
	}
}

func TestResourceResourceGrantDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeResourceGrants{deleteErr: fmt.Errorf("delete grant: %w", gql.ErrNotFound)}
	pc := &ProviderClient{ResourceGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceResourceGrant().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceResourceGrantDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceResourceGrantSchema(t *testing.T) {
	r := resourceResourceGrant()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	for _, field := range []string{"resource_id", "recipient_conditions"} {
		s := r.Schema[field]
		if s == nil || !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew (grants are immutable)", field)
		}
	}
	if r.UpdateContext != nil {
		t.Error("grants are immutable; there should be no UpdateContext")
	}
}

func TestSplitGrantImportID(t *testing.T) {
	cases := map[string]struct {
		parentID, grantID string
		wantErr           bool
	}{
		"resource-1/grant-1": {parentID: "resource-1", grantID: "grant-1"},
		// Parent IDs with slashes split on the LAST slash — grant IDs are
		// UUIDs and never contain one.
		"org/repo-name/grant-2": {parentID: "org/repo-name", grantID: "grant-2"},
		"no-slash":              {wantErr: true},
		"/grant-only":           {wantErr: true},
		"parent-only/":          {wantErr: true},
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			parentID, grantID, err := splitGrantImportID(raw, "resource")
			if want.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got parent=%q grant=%q", raw, parentID, grantID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parentID != want.parentID || grantID != want.grantID {
				t.Errorf("got (%q, %q), want (%q, %q)", parentID, grantID, want.parentID, want.grantID)
			}
		})
	}
}
