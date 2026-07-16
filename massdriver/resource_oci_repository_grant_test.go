package massdriver

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/ocirepos"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

type fakeOciRepoGrants struct {
	createResp *ocirepos.Grant
	createErr  error
	deleteErr  error

	grants  []ocirepos.Grant
	iterErr error

	createRepoID string
	createInput  ocirepos.CreateGrantInput
	iterRepoID   string
	deleteID     string

	createCalls, iterCalls, deleteCalls int
}

func (f *fakeOciRepoGrants) CreateGrant(_ context.Context, repoID string, input ocirepos.CreateGrantInput) (*ocirepos.Grant, error) {
	f.createRepoID = repoID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}

func (f *fakeOciRepoGrants) DeleteGrant(_ context.Context, grantID string) error {
	f.deleteID = grantID
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeOciRepoGrants) IterGrants(_ context.Context, repoID string, _ ocirepos.ListGrantsInput) iter.Seq2[ocirepos.Grant, error] {
	f.iterRepoID = repoID
	f.iterCalls++
	return func(yield func(ocirepos.Grant, error) bool) {
		if f.iterErr != nil {
			yield(ocirepos.Grant{}, f.iterErr)
			return
		}
		for _, g := range f.grants {
			if !yield(g, nil) {
				return
			}
		}
	}
}

func TestResourceOciRepositoryGrantCreate(t *testing.T) {
	grant := ocirepos.Grant{
		ID:                  "grant-1",
		Action:              "repo:pull",
		RecipientConditions: types.PolicyConditions{"team": {"eng"}},
	}
	fake := &fakeOciRepoGrants{createResp: &grant, grants: []ocirepos.Grant{grant}}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id":        "aws-aurora-postgres",
		"recipient_conditions": `{"team":["eng"]}`,
	})

	if diags := resourceOciRepositoryGrantCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "grant-1" {
		t.Errorf("got id %q, want grant-1", rd.Id())
	}
	if fake.createRepoID != "aws-aurora-postgres" {
		t.Errorf("got CreateGrant repoID %q, want aws-aurora-postgres", fake.createRepoID)
	}
	in := fake.createInput
	if in.Action != "repo:pull" {
		t.Errorf("got Action %q, want the hardcoded repo:pull", in.Action)
	}
	if got := in.RecipientConditions["team"]; len(got) != 1 || got[0] != "eng" {
		t.Errorf("got RecipientConditions[team] %v, want [eng]", got)
	}
}

func TestResourceOciRepositoryGrantCreateWildcardConditions(t *testing.T) {
	grant := ocirepos.Grant{ID: "grant-2", Action: "repo:pull", RecipientConditions: nil}
	fake := &fakeOciRepoGrants{createResp: &grant, grants: []ocirepos.Grant{grant}}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id":        "aws-aurora-postgres",
		"recipient_conditions": "*",
	})

	if diags := resourceOciRepositoryGrantCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.createInput.RecipientConditions != nil {
		t.Errorf("got RecipientConditions %v, want nil (wildcard sentinel)", fake.createInput.RecipientConditions)
	}
}

func TestResourceOciRepositoryGrantCreateRejectsInvalidConditionsJSON(t *testing.T) {
	fake := &fakeOciRepoGrants{}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id":        "aws-aurora-postgres",
		"recipient_conditions": `not json`,
	})

	diags := resourceOciRepositoryGrantCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected JSON parse error, got none")
	}
	if fake.createCalls != 0 {
		t.Errorf("CreateGrant should not fire on parse error; got %d calls", fake.createCalls)
	}
}

func TestResourceOciRepositoryGrantRead(t *testing.T) {
	fake := &fakeOciRepoGrants{grants: []ocirepos.Grant{
		{ID: "grant-other", Action: "repo:pull", RecipientConditions: nil},
		{ID: "grant-1", Action: "repo:pull", RecipientConditions: types.PolicyConditions{"team": {"eng"}}},
	}}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id": "aws-aurora-postgres",
	})
	rd.SetId("grant-1")

	if diags := resourceOciRepositoryGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.iterRepoID != "aws-aurora-postgres" {
		t.Errorf("got IterGrants repoID %q, want aws-aurora-postgres", fake.iterRepoID)
	}
	if got := rd.Get("recipient_conditions").(string); got != `{"team":["eng"]}` {
		t.Errorf("got recipient_conditions %q, want plain JSON object", got)
	}
}

func TestResourceOciRepositoryGrantReadClearsWhenGrantMissing(t *testing.T) {
	fake := &fakeOciRepoGrants{grants: []ocirepos.Grant{{ID: "grant-other"}}}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id": "aws-aurora-postgres",
	})
	rd.SetId("gone")

	if diags := resourceOciRepositoryGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("missing grant should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared when grant is absent; got %q", rd.Id())
	}
}

func TestResourceOciRepositoryGrantReadClearsWhenParentNotFound(t *testing.T) {
	fake := &fakeOciRepoGrants{iterErr: fmt.Errorf("list oci repo aws-aurora-postgres grants: %w", gql.ErrNotFound)}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{
		"repository_id": "aws-aurora-postgres",
	})
	rd.SetId("grant-1")

	if diags := resourceOciRepositoryGrantRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("parent not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on parent not-found; got %q", rd.Id())
	}
}

func TestResourceOciRepositoryGrantDelete(t *testing.T) {
	fake := &fakeOciRepoGrants{}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{})
	rd.SetId("grant-1")

	if diags := resourceOciRepositoryGrantDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "grant-1" {
		t.Errorf("got deleteID %q, want grant-1", fake.deleteID)
	}
}

func TestResourceOciRepositoryGrantDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeOciRepoGrants{deleteErr: fmt.Errorf("delete grant: %w", gql.ErrNotFound)}
	pc := &ProviderClient{OciRepoGrants: fake}

	rd := schema.TestResourceDataRaw(t, resourceOciRepositoryGrant().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceOciRepositoryGrantDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func TestResourceOciRepositoryGrantSchema(t *testing.T) {
	r := resourceOciRepositoryGrant()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	for _, field := range []string{"repository_id", "recipient_conditions"} {
		s := r.Schema[field]
		if s == nil || !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew (grants are immutable)", field)
		}
	}
	if r.UpdateContext != nil {
		t.Error("grants are immutable; there should be no UpdateContext")
	}
}
