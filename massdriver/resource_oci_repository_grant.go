package massdriver

import (
	"context"
	"errors"
	"iter"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/ocirepos"
)

// ociRepoGrantsAPI is the slice of *platform/ocirepos.Service this resource
// calls. The SDK exposes grants on the same service as repositories; the
// provider keeps a separate interface so tests can fake grants independently.
type ociRepoGrantsAPI interface {
	CreateGrant(ctx context.Context, repoID string, input ocirepos.CreateGrantInput) (*ocirepos.Grant, error)
	DeleteGrant(ctx context.Context, grantID string) error
	IterGrants(ctx context.Context, repoID string, input ocirepos.ListGrantsInput) iter.Seq2[ocirepos.Grant, error]
}

var _ ociRepoGrantsAPI = (*ocirepos.Service)(nil)

// ociRepoGrantAction is the action every repository grant carries. It's the
// only grantable repo action, so the provider sets it rather than exposing
// an `action` field; if the platform ever adds more, they'll be supported
// explicitly in a future provider version.
const ociRepoGrantAction = "repo:pull"

func resourceOciRepositoryGrant() *schema.Resource {
	return &schema.Resource{
		Description: "A sharing grant on an OCI repository. Each grant shares the repository (granting `repo:pull`) with recipient **projects** matching `recipient_conditions` (sharing implies the repository is visible to those recipients). Creating or deleting grants requires `repo:grant` on the repository. Grants are immutable — every change replaces the grant.",

		CreateContext: resourceOciRepositoryGrantCreate,
		ReadContext:   resourceOciRepositoryGrantRead,
		DeleteContext: resourceOciRepositoryGrantDelete,

		// Grants have no get-by-ID API — Read lists the parent repository's
		// grants — so import takes the composite `<repository_id>/<grant_id>`.
		Importer: &schema.ResourceImporter{
			StateContext: func(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
				repoID, grantID, err := splitGrantImportID(d.Id(), "repository")
				if err != nil {
					return nil, err
				}
				d.Set("repository_id", repoID)
				d.SetId(grantID)
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"repository_id": {
				Description: "ID of the OCI repository being shared (a `massdriver_oci_repository` ID). Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"recipient_conditions": {
				Description: "Which recipient projects qualify: either the literal `\"*\"` (wildcard — every project in the org) or a JSON-encoded object of attribute conditions (e.g., `jsonencode({team = [\"eng\"]})`). Per attribute, an empty list matches any value of that attribute. Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceOciRepositoryGrantCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	conditions, err := decodePolicyConditions(d.Get("recipient_conditions").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	grant, err := pc.OciRepoGrants.CreateGrant(ctx, d.Get("repository_id").(string), ocirepos.CreateGrantInput{
		Action:              ociRepoGrantAction,
		RecipientConditions: conditions,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(grant.ID)
	return resourceOciRepositoryGrantRead(ctx, d, meta)
}

func resourceOciRepositoryGrantRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	seq := pc.OciRepoGrants.IterGrants(ctx, d.Get("repository_id").(string), ocirepos.ListGrantsInput{})
	grant, err := findGrant(seq, d.Id())
	if err != nil {
		// Parent repository gone — the grant went with it.
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

func resourceOciRepositoryGrantDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := pc.OciRepoGrants.DeleteGrant(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
