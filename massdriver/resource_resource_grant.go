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
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/resources"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/types"
)

// resourceGrantsAPI is the slice of *platform/resources.Service this resource
// calls. The SDK exposes grants on the same service as resources; the provider
// keeps a separate interface so tests can fake grants independently.
type resourceGrantsAPI interface {
	CreateGrant(ctx context.Context, resourceID string, input resources.CreateGrantInput) (*resources.Grant, error)
	DeleteGrant(ctx context.Context, grantID string) error
	IterGrants(ctx context.Context, resourceID string, input resources.ListGrantsInput) iter.Seq2[resources.Grant, error]
}

var _ resourceGrantsAPI = (*resources.Service)(nil)

// resourceGrantAction is the action every resource grant carries. It's the
// only grantable resource action, so the provider sets it rather than
// exposing an `action` field; if the platform ever adds more, they'll be
// supported explicitly in a future provider version.
const resourceGrantAction = "resource:export"

func resourceResourceGrant() *schema.Resource {
	return &schema.Resource{
		Description: "A sharing grant on a resource. Each grant shares the resource (granting `resource:export`) with recipient **environments** matching `recipient_conditions` (sharing implies the resource is visible to those recipients). Creating or deleting grants requires `resource:grant` on the resource. Grants are immutable — every change replaces the grant.",

		CreateContext: resourceResourceGrantCreate,
		ReadContext:   resourceResourceGrantRead,
		DeleteContext: resourceResourceGrantDelete,

		// Grants have no get-by-ID API — Read lists the parent resource's
		// grants — so import takes the composite `<resource_id>/<grant_id>`.
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
				resourceID, grantID, err := splitGrantImportID(d.Id(), "resource")
				if err != nil {
					return nil, err
				}
				d.Set("resource_id", resourceID)
				d.SetId(grantID)
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"resource_id": {
				Description: "ID of the resource being shared (a `massdriver_imported_resource` or `massdriver_resource` ID). Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"recipient_conditions": {
				Description: "Which recipient environments qualify: either the literal `\"*\"` (wildcard — every environment in the org) or a JSON-encoded object of attribute conditions (e.g., `jsonencode({team = [\"eng\"]})`). Per attribute, an empty list matches any value of that attribute. Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceResourceGrantCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	conditions, err := decodePolicyConditions(d.Get("recipient_conditions").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	grant, err := pc.ResourceGrants.CreateGrant(ctx, d.Get("resource_id").(string), resources.CreateGrantInput{
		Action:              resourceGrantAction,
		RecipientConditions: conditions,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(grant.ID)
	return resourceResourceGrantRead(ctx, d, meta)
}

func resourceResourceGrantRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	seq := pc.ResourceGrants.IterGrants(ctx, d.Get("resource_id").(string), resources.ListGrantsInput{})
	grant, err := findGrant(seq, d.Id())
	if err != nil {
		// Parent resource gone — the grant went with it.
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	if grant == nil {
		d.SetId("")
		return nil
	}

	d.Set("recipient_conditions", encodePolicyConditions(grant.RecipientConditions))
	return nil
}

func resourceResourceGrantDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := pc.ResourceGrants.DeleteGrant(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// findGrant scans a grant iterator for the given ID. Returns (nil, nil) when
// the parent exists but no grant matches — the caller clears state. A missing
// parent surfaces as the iterator's error (wrapping gql.ErrNotFound).
func findGrant(seq iter.Seq2[types.Grant, error], grantID string) (*types.Grant, error) {
	for grant, err := range seq {
		if err != nil {
			return nil, err
		}
		if grant.ID == grantID {
			return &grant, nil
		}
	}
	return nil, nil
}

// splitGrantImportID parses the composite `<parent_id>/<grant_id>` import ID
// used by the grant resources. Split on the last slash: grant IDs are UUIDs
// and never contain one.
func splitGrantImportID(raw, parentNoun string) (parentID, grantID string, err error) {
	i := strings.LastIndex(raw, "/")
	if i <= 0 || i == len(raw)-1 {
		return "", "", fmt.Errorf("unexpected import ID %q: expected <%s_id>/<grant_id>", raw, parentNoun)
	}
	return raw[:i], raw[i+1:], nil
}
