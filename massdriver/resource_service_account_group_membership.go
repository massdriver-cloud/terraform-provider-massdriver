package massdriver

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/groups"
)

// serviceAccountMembershipsAPI is the slice of *groups.Service this resource
// calls. Membership lives on the groups service; the provider keeps a
// separate interface so tests can fake it independently of groups.
type serviceAccountMembershipsAPI interface {
	AddServiceAccount(ctx context.Context, groupID, serviceAccountID string) error
	RemoveServiceAccount(ctx context.Context, groupID, serviceAccountID string) error
	IterServiceAccounts(ctx context.Context, groupID string, input groups.ListServiceAccountsInput) iter.Seq2[groups.ServiceAccount, error]
}

var _ serviceAccountMembershipsAPI = (*groups.Service)(nil)

func resourceServiceAccountGroupMembership() *schema.Resource {
	return &schema.Resource{
		Description: "Adds a service account to a group, granting it the group's permissions. A service account can belong to several groups; its effective permissions are the union. Memberships are immutable — changing either ID replaces the membership.",

		CreateContext: resourceServiceAccountGroupMembershipCreate,
		ReadContext:   resourceServiceAccountGroupMembershipRead,
		DeleteContext: resourceServiceAccountGroupMembershipDelete,

		// The ID is the composite `<group_id>/<service_account_id>`, so import
		// takes that form too.
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
				groupID, serviceAccountID, err := splitMembershipID(d.Id())
				if err != nil {
					return nil, err
				}
				d.Set("group_id", groupID)
				d.Set("service_account_id", serviceAccountID)
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"group_id": {
				Description: "ID of the group (a `massdriver_group` ID, or a built-in group such as `organization_admin`). Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"service_account_id": {
				Description: "ID of the service account to add (a `massdriver_service_account` ID). Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceServiceAccountGroupMembershipCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	groupID := d.Get("group_id").(string)
	serviceAccountID := d.Get("service_account_id").(string)

	if err := pc.ServiceAccountMemberships.AddServiceAccount(ctx, groupID, serviceAccountID); err != nil {
		// The API rejects a duplicate add without saying why, so check
		// whether that's what happened and point at import.
		if member, lerr := isGroupMember(ctx, pc, groupID, serviceAccountID); lerr == nil && member {
			return diag.Diagnostics{{
				Severity: diag.Error,
				Summary:  "Service account is already a member of this group",
				Detail: fmt.Sprintf("Service account %s already belongs to group %s outside terraform. To manage it, import it:\n\n"+
					"  terraform import <resource address> %s/%s", serviceAccountID, groupID, groupID, serviceAccountID),
			}}
		}
		return diag.FromErr(err)
	}

	d.SetId(groupID + "/" + serviceAccountID)
	return resourceServiceAccountGroupMembershipRead(ctx, d, meta)
}

func resourceServiceAccountGroupMembershipRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	found, err := isGroupMember(ctx, pc, d.Get("group_id").(string), d.Get("service_account_id").(string))
	if err != nil {
		// Group gone — the membership went with it.
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	if !found {
		d.SetId("")
	}
	return nil
}

func resourceServiceAccountGroupMembershipDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	groupID := d.Get("group_id").(string)
	serviceAccountID := d.Get("service_account_id").(string)

	if err := pc.ServiceAccountMemberships.RemoveServiceAccount(ctx, groupID, serviceAccountID); err != nil {
		if !errors.Is(err, gql.ErrNotFound) && !membershipGone(ctx, pc, groupID, serviceAccountID) {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return nil
}

// membershipGone reports whether the membership has already been removed,
// along with its group. Removing a non-member fails without saying why, so a
// failed remove is checked this way before it's reported.
func membershipGone(ctx context.Context, pc *ProviderClient, groupID, serviceAccountID string) bool {
	member, err := isGroupMember(ctx, pc, groupID, serviceAccountID)
	if err != nil {
		return errors.Is(err, gql.ErrNotFound)
	}
	return !member
}

// isGroupMember walks the group's service accounts looking for
// serviceAccountID, stopping at the first match.
func isGroupMember(ctx context.Context, pc *ProviderClient, groupID, serviceAccountID string) (bool, error) {
	seq := pc.ServiceAccountMemberships.IterServiceAccounts(ctx, groupID, groups.ListServiceAccountsInput{})
	return containsServiceAccount(seq, serviceAccountID)
}

func containsServiceAccount(seq iter.Seq2[groups.ServiceAccount, error], id string) (bool, error) {
	for sa, err := range seq {
		if err != nil {
			return false, err
		}
		if sa.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// splitMembershipID parses the composite `<group_id>/<service_account_id>`
// ID. Both halves are UUIDs or slugs and never contain a slash.
func splitMembershipID(raw string) (groupID, serviceAccountID string, err error) {
	groupID, serviceAccountID, ok := strings.Cut(raw, "/")
	if !ok || groupID == "" || serviceAccountID == "" || strings.Contains(serviceAccountID, "/") {
		return "", "", fmt.Errorf("unexpected ID %q: expected <group_id>/<service_account_id>", raw)
	}
	return groupID, serviceAccountID, nil
}
