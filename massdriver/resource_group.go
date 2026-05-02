package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

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
			"role": {
				Description: "Access level granted by membership. New custom groups always come back as `CUSTOM`; the field is exposed for clarity and importing built-in groups.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceGroupCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	group, err := api.CreateGroup(ctx, client, api.CreateGroupInput{
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
	client := meta.(*ProviderClient).Client

	group, err := api.GetGroup(ctx, client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", group.Name)
	d.Set("description", group.Description)
	d.Set("role", group.Role)
	return nil
}

func resourceGroupUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	_, err := api.UpdateGroup(ctx, client, d.Id(), api.UpdateGroupInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceGroupRead(ctx, d, meta)
}

func resourceGroupDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteGroup(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
