package massdriver

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/groups"
)

type fakeServiceAccountMemberships struct {
	addErr, removeErr error

	members []groups.ServiceAccount
	iterErr error

	addGroupID, addServiceAccountID       string
	removeGroupID, removeServiceAccountID string
	iterGroupID                           string

	addCalls, removeCalls, iterCalls int
}

func (f *fakeServiceAccountMemberships) AddServiceAccount(_ context.Context, groupID, serviceAccountID string) error {
	f.addGroupID, f.addServiceAccountID = groupID, serviceAccountID
	f.addCalls++
	return f.addErr
}

func (f *fakeServiceAccountMemberships) RemoveServiceAccount(_ context.Context, groupID, serviceAccountID string) error {
	f.removeGroupID, f.removeServiceAccountID = groupID, serviceAccountID
	f.removeCalls++
	return f.removeErr
}

func (f *fakeServiceAccountMemberships) IterServiceAccounts(_ context.Context, groupID string, _ groups.ListServiceAccountsInput) iter.Seq2[groups.ServiceAccount, error] {
	f.iterGroupID = groupID
	f.iterCalls++
	return func(yield func(groups.ServiceAccount, error) bool) {
		if f.iterErr != nil {
			yield(groups.ServiceAccount{}, f.iterErr)
			return
		}
		for _, sa := range f.members {
			if !yield(sa, nil) {
				return
			}
		}
	}
}

func membershipData(t *testing.T) *schema.ResourceData {
	t.Helper()
	return schema.TestResourceDataRaw(t, resourceServiceAccountGroupMembership().Schema, map[string]any{
		"group_id":           "group-1",
		"service_account_id": "sa-1",
	})
}

func TestResourceServiceAccountGroupMembershipCreate(t *testing.T) {
	fake := &fakeServiceAccountMemberships{members: []groups.ServiceAccount{{ID: "sa-1"}}}
	pc := &ProviderClient{ServiceAccountMemberships: fake}

	rd := membershipData(t)
	if diags := resourceServiceAccountGroupMembershipCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if fake.addGroupID != "group-1" || fake.addServiceAccountID != "sa-1" {
		t.Errorf("got add(%q, %q), want add(group-1, sa-1)", fake.addGroupID, fake.addServiceAccountID)
	}
	if rd.Id() != "group-1/sa-1" {
		t.Errorf("got id %q, want group-1/sa-1", rd.Id())
	}
}

func TestResourceServiceAccountGroupMembershipCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeServiceAccountMemberships{addErr: fmt.Errorf("add service account to group: %w", gql.ErrForbidden)}
	pc := &ProviderClient{ServiceAccountMemberships: fake}

	rd := membershipData(t)
	if diags := resourceServiceAccountGroupMembershipCreate(t.Context(), rd, pc); !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceServiceAccountGroupMembershipRead(t *testing.T) {
	tests := []struct {
		name     string
		fake     *fakeServiceAccountMemberships
		wantID   string
		wantDiag bool
	}{
		{
			name:   "member present",
			fake:   &fakeServiceAccountMemberships{members: []groups.ServiceAccount{{ID: "other"}, {ID: "sa-1"}}},
			wantID: "group-1/sa-1",
		},
		{
			name:   "removed out of band",
			fake:   &fakeServiceAccountMemberships{members: []groups.ServiceAccount{{ID: "other"}}},
			wantID: "",
		},
		{
			name:   "group deleted",
			fake:   &fakeServiceAccountMemberships{iterErr: fmt.Errorf("list: %w", gql.ErrNotFound)},
			wantID: "",
		},
		{
			name:     "transient failure",
			fake:     &fakeServiceAccountMemberships{iterErr: fmt.Errorf("list: boom")},
			wantID:   "group-1/sa-1",
			wantDiag: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rd := membershipData(t)
			rd.SetId("group-1/sa-1")

			diags := resourceServiceAccountGroupMembershipRead(t.Context(), rd, &ProviderClient{ServiceAccountMemberships: tc.fake})
			if diags.HasError() != tc.wantDiag {
				t.Fatalf("got diags %v, wantDiag %v", diags, tc.wantDiag)
			}
			if rd.Id() != tc.wantID {
				t.Errorf("got id %q, want %q", rd.Id(), tc.wantID)
			}
			if tc.fake.iterGroupID != "group-1" {
				t.Errorf("listed group %q, want group-1", tc.fake.iterGroupID)
			}
		})
	}
}

func TestResourceServiceAccountGroupMembershipDelete(t *testing.T) {
	for name, removeErr := range map[string]error{
		"success":   nil,
		"not found": fmt.Errorf("remove: %w", gql.ErrNotFound),
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeServiceAccountMemberships{removeErr: removeErr}
			rd := membershipData(t)
			rd.SetId("group-1/sa-1")

			if diags := resourceServiceAccountGroupMembershipDelete(t.Context(), rd, &ProviderClient{ServiceAccountMemberships: fake}); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if fake.removeGroupID != "group-1" || fake.removeServiceAccountID != "sa-1" {
				t.Errorf("got remove(%q, %q), want remove(group-1, sa-1)", fake.removeGroupID, fake.removeServiceAccountID)
			}
			if rd.Id() != "" {
				t.Errorf("ID should be cleared, got %q", rd.Id())
			}
		})
	}
}

func TestResourceServiceAccountGroupMembershipImport(t *testing.T) {
	r := resourceServiceAccountGroupMembership()

	rd := r.TestResourceData()
	rd.SetId("group-1/sa-1")
	got, err := r.Importer.StateContext(t.Context(), rd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g := got[0].Get("group_id").(string); g != "group-1" {
		t.Errorf("got group_id %q, want group-1", g)
	}
	if s := got[0].Get("service_account_id").(string); s != "sa-1" {
		t.Errorf("got service_account_id %q, want sa-1", s)
	}

	for _, bad := range []string{"sa-1", "/sa-1", "group-1/", "a/b/c"} {
		rd := r.TestResourceData()
		rd.SetId(bad)
		if _, err := r.Importer.StateContext(t.Context(), rd, nil); err == nil {
			t.Errorf("import ID %q should be rejected", bad)
		}
	}
}

func TestResourceServiceAccountGroupMembershipSchema(t *testing.T) {
	r := resourceServiceAccountGroupMembership()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	for _, k := range []string{"group_id", "service_account_id"} {
		if s := r.Schema[k]; s == nil || !s.Required || !s.ForceNew {
			t.Errorf("%s should be Required+ForceNew", k)
		}
	}
}
