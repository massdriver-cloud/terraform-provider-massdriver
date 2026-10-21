package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/serviceaccounts"
)

type fakeServiceAccounts struct {
	getResp, updateResp, deleteResp *serviceaccounts.ServiceAccount
	createResp                      *serviceaccounts.ServiceAccountWithToken
	getErr, createErr, updateErr    error
	deleteErr                       error

	createInput serviceaccounts.CreateInput
	updateID    string
	updateInput serviceaccounts.UpdateInput
	deleteID    string

	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeServiceAccounts) Get(_ context.Context, _ string) (*serviceaccounts.ServiceAccount, error) {
	f.getCalls++
	return f.getResp, f.getErr
}
func (f *fakeServiceAccounts) Create(_ context.Context, input serviceaccounts.CreateInput) (*serviceaccounts.ServiceAccountWithToken, error) {
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}
func (f *fakeServiceAccounts) Update(_ context.Context, id string, input serviceaccounts.UpdateInput) (*serviceaccounts.ServiceAccount, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}
func (f *fakeServiceAccounts) Delete(_ context.Context, id string) (*serviceaccounts.ServiceAccount, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

func TestResourceServiceAccountCreate(t *testing.T) {
	sa := serviceaccounts.ServiceAccount{ID: "sa-1", Name: "ci-deployer", Description: "GitHub Actions"}
	fake := &fakeServiceAccounts{
		createResp: &serviceaccounts.ServiceAccountWithToken{ServiceAccount: sa, DefaultToken: "mds_secret", DefaultTokenID: "tok-1"},
		getResp:    &sa,
	}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{
		"name":                     "ci-deployer",
		"description":              "GitHub Actions",
		"token_expiration_minutes": 525600,
	})

	if diags := resourceServiceAccountCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "sa-1" {
		t.Errorf("got id %q, want sa-1", rd.Id())
	}
	want := serviceaccounts.CreateInput{Name: "ci-deployer", Description: "GitHub Actions", DefaultAccessTokenExpirationInMinutes: 525600}
	if fake.createInput != want {
		t.Errorf("got create input %+v, want %+v", fake.createInput, want)
	}
	if got := rd.Get("token").(string); got != "mds_secret" {
		t.Errorf("got token %q, want mds_secret", got)
	}
	if got := rd.Get("token_id").(string); got != "tok-1" {
		t.Errorf("got token_id %q, want tok-1", got)
	}
}

// Create must not Read back: a not-found there would clear the ID and lose
// the only copy of the token.
func TestResourceServiceAccountCreateDoesNotReadBack(t *testing.T) {
	fake := &fakeServiceAccounts{
		createResp: &serviceaccounts.ServiceAccountWithToken{
			ServiceAccount: serviceaccounts.ServiceAccount{ID: "sa-1", Name: "ci-deployer", Description: "from create"},
			DefaultToken:   "mds_secret",
		},
		getErr: fmt.Errorf("get service account: %w", gql.ErrNotFound),
	}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{
		"name":                     "ci-deployer",
		"token_expiration_minutes": 60,
	})

	if diags := resourceServiceAccountCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.getCalls != 0 {
		t.Errorf("create made %d Get calls, want 0", fake.getCalls)
	}
	if rd.Id() != "sa-1" || rd.Get("token").(string) != "mds_secret" {
		t.Errorf("got id %q, token %q; want sa-1 and the token", rd.Id(), rd.Get("token"))
	}
	if got := rd.Get("description").(string); got != "from create" {
		t.Errorf("got description %q, want it from the create response", got)
	}
}

func TestResourceServiceAccountCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeServiceAccounts{createErr: fmt.Errorf("create service account: name must be unique")}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{
		"name":                     "ci-deployer",
		"token_expiration_minutes": 60,
	})

	diags := resourceServiceAccountCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(diags[0].Summary, "name must be unique") {
		t.Errorf("upstream error %q should be surfaced", diags[0].Summary)
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

// Read must leave the token alone: the API never returns it after creation,
// so overwriting it would wipe the only copy from state.
func TestResourceServiceAccountReadPreservesToken(t *testing.T) {
	pc := &ProviderClient{ServiceAccounts: &fakeServiceAccounts{
		getResp: &serviceaccounts.ServiceAccount{ID: "sa-1", Name: "renamed", Description: "from the server"},
	}}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{})
	rd.SetId("sa-1")
	rd.Set("token", "mds_secret")

	if diags := resourceServiceAccountRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("name").(string) != "renamed" || rd.Get("description").(string) != "from the server" {
		t.Errorf("got name %q, description %q", rd.Get("name"), rd.Get("description"))
	}
	if rd.Get("token").(string) != "mds_secret" {
		t.Errorf("token should survive Read, got %q", rd.Get("token"))
	}
}

func TestResourceServiceAccountReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{ServiceAccounts: &fakeServiceAccounts{
		getErr: fmt.Errorf("get service account: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceServiceAccountRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceServiceAccountUpdate(t *testing.T) {
	resp := &serviceaccounts.ServiceAccount{ID: "sa-1", Name: "renamed", Description: "updated"}
	fake := &fakeServiceAccounts{updateResp: resp, getResp: resp}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{
		"name":                     "renamed",
		"description":              "updated",
		"token_expiration_minutes": 60,
	})
	rd.SetId("sa-1")

	if diags := resourceServiceAccountUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "sa-1" {
		t.Errorf("got updateID %q, want sa-1", fake.updateID)
	}
	if in := fake.updateInput; in.Name != "renamed" || in.Description != "updated" {
		t.Errorf("got input %+v", in)
	}
}

func TestResourceServiceAccountDelete(t *testing.T) {
	fake := &fakeServiceAccounts{deleteResp: &serviceaccounts.ServiceAccount{ID: "sa-1"}}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{})
	rd.SetId("sa-1")

	if diags := resourceServiceAccountDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "sa-1" {
		t.Errorf("got deleteID %q, want sa-1", fake.deleteID)
	}
}

func TestResourceServiceAccountDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeServiceAccounts{deleteErr: fmt.Errorf("delete service account: %w", gql.ErrNotFound)}
	pc := &ProviderClient{ServiceAccounts: fake}

	rd := schema.TestResourceDataRaw(t, resourceServiceAccount().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceServiceAccountDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

func serviceAccountDiff(t *testing.T, stateAttrs map[string]string, cfg map[string]any) *terraform.InstanceDiff {
	t.Helper()
	state := &terraform.InstanceState{ID: "sa-1", Attributes: stateAttrs}
	diff, err := resourceServiceAccount().Diff(t.Context(), state, terraform.NewResourceConfigRaw(cfg), nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	return diff
}

// Imported service accounts have no expiration in state; that mustn't plan a
// replacement.
func TestResourceServiceAccountIgnoresExpirationAfterImport(t *testing.T) {
	diff := serviceAccountDiff(t,
		map[string]string{"id": "sa-1", "name": "ci-deployer", "description": ""},
		map[string]any{"name": "ci-deployer", "token_expiration_minutes": 60},
	)
	if diff != nil && diff.RequiresNew() {
		t.Errorf("imported service account should not be replaced; got %+v", diff.Attributes)
	}
}

func TestResourceServiceAccountExpirationChangeReplaces(t *testing.T) {
	diff := serviceAccountDiff(t,
		map[string]string{"id": "sa-1", "name": "ci-deployer", "description": "", "token_expiration_minutes": "60"},
		map[string]any{"name": "ci-deployer", "token_expiration_minutes": 120},
	)
	if diff == nil || !diff.RequiresNew() {
		t.Errorf("changing token_expiration_minutes should replace; got %+v", diff)
	}
}

func TestResourceServiceAccountSchema(t *testing.T) {
	r := resourceServiceAccount()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	if tok := r.Schema["token"]; tok == nil || !tok.Computed || !tok.Sensitive {
		t.Error("token should be Computed+Sensitive")
	}
	if exp := r.Schema["token_expiration_minutes"]; exp == nil || !exp.Required || !exp.ForceNew {
		t.Error("token_expiration_minutes should be Required+ForceNew")
	}
}
