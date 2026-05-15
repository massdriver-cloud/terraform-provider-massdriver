package massdriver

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/groups"
)

// groupsAPI is the slice of *groups.Service this resource calls.
type groupsAPI interface {
	Get(ctx context.Context, id string) (*groups.Group, error)
	Create(ctx context.Context, input groups.CreateInput) (*groups.Group, error)
	Update(ctx context.Context, id string, input groups.UpdateInput) (*groups.Group, error)
	Delete(ctx context.Context, id string) (*groups.Group, error)
}

var _ groupsAPI = (*groups.Service)(nil)

func resourceGroup() *schema.Resource {
	return &schema.Resource{
		Description: "A custom Massdriver group. Built-in groups (`organization_admin`, `organization_viewer`) are managed by the platform and cannot be created or destroyed via terraform.",

		CreateContext: resourceGroupCreate,
		ReadContext:   resourceGroupRead,
		UpdateContext: resourceGroupUpdate,
		DeleteContext: resourceGroupDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Human-readable name for the group. Must be unique within the organization.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"description": {
				Description: "Optional description of the group's purpose. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

func resourceGroupCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	group, err := pc.Groups.Create(ctx, groups.CreateInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(group.ID)
	return resourceGroupRead(ctx, d, meta)
}

func resourceGroupRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	group, err := pc.Groups.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("name", group.Name)
	d.Set("description", group.Description)
	return nil
}

func resourceGroupUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Groups.Update(ctx, d.Id(), groups.UpdateInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceGroupRead(ctx, d, meta)
}

func resourceGroupDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Groups.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
