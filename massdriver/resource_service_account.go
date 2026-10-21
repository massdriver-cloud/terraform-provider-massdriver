package massdriver

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/serviceaccounts"
)

// serviceAccountsAPI is the slice of *serviceaccounts.Service this resource
// calls.
type serviceAccountsAPI interface {
	Get(ctx context.Context, id string) (*serviceaccounts.ServiceAccount, error)
	Create(ctx context.Context, input serviceaccounts.CreateInput) (*serviceaccounts.ServiceAccountWithToken, error)
	Update(ctx context.Context, id string, input serviceaccounts.UpdateInput) (*serviceaccounts.ServiceAccount, error)
	Delete(ctx context.Context, id string) (*serviceaccounts.ServiceAccount, error)
}

var _ serviceAccountsAPI = (*serviceaccounts.Service)(nil)

// maxServiceAccountTokenMinutes is the platform's cap on service account
// token lifetime (10 years).
const maxServiceAccountTokenMinutes = 5_256_000

func resourceServiceAccount() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver service account — a non-human identity for CI/CD pipelines, scripts, and other API clients. " +
			"The platform issues one access token when the service account is created; it is exposed as the sensitive `token` attribute and cannot be retrieved again. " +
			"To rotate the token, replace the resource (e.g., `terraform apply -replace`, or `replace_triggered_by` on a `time_rotating`). " +
			"A new service account has no permissions until it is added to a group with `massdriver_service_account_group_membership`.",

		CreateContext: resourceServiceAccountCreate,
		ReadContext:   resourceServiceAccountRead,
		UpdateContext: resourceServiceAccountUpdate,
		DeleteContext: resourceServiceAccountDelete,

		// The token is only returned at creation, so an imported service
		// account has an empty `token`.
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Human-readable name, shown in the UI, logs, and audit trail.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"description": {
				Description: "Optional description of what the service account is used for. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"token_expiration_minutes": {
				Description: "How long the token issued at creation stays valid, in minutes. Max 5,256,000 (10 years). Changing it replaces the service account, issuing a new token. Ignored after import, since the platform doesn't report it.",
				Type:        schema.TypeInt,
				Required:    true,
				ForceNew:    true,
				// The platform doesn't return the expiration, so an imported
				// service account has none in state; without this, the first
				// plan after import would replace it.
				DiffSuppressFunc: func(_, old, _ string, d *schema.ResourceData) bool {
					return d.Id() != "" && (old == "" || old == "0")
				},
				ValidateFunc: validation.IntBetween(1, maxServiceAccountTokenMinutes),
			},
			"token": {
				Description: "The service account's bearer token. Only available on the resource that created the service account; empty after import.",
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
			},
			"token_id": {
				Description: "ID of the token issued at creation. Empty after import.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceServiceAccountCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	sa, err := pc.ServiceAccounts.Create(ctx, serviceaccounts.CreateInput{
		Name:                                  d.Get("name").(string),
		Description:                           d.Get("description").(string),
		DefaultAccessTokenExpirationInMinutes: d.Get("token_expiration_minutes").(int),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(sa.ID)
	d.Set("token", sa.DefaultToken)
	d.Set("token_id", sa.DefaultTokenID)
	return resourceServiceAccountRead(ctx, d, meta)
}

func resourceServiceAccountRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	sa, err := pc.ServiceAccounts.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("name", sa.Name)
	d.Set("description", sa.Description)
	return nil
}

func resourceServiceAccountUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.ServiceAccounts.Update(ctx, d.Id(), serviceaccounts.UpdateInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceServiceAccountRead(ctx, d, meta)
}

// Deleting a service account revokes all of its tokens and removes its group
// memberships.
func resourceServiceAccountDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.ServiceAccounts.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
