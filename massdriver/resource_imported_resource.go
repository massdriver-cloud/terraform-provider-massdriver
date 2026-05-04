package massdriver

import (
	"context"
	"encoding/json"
	"fmt"

	"terraform-provider-massdriver/internal/api"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceImportedResource() *schema.Resource {
	return &schema.Resource{
		Description: `Registers an existing infrastructure or cloud asset with Massdriver as a connectable resource — for example credentials, or an existing cloud resource not managed by Massdriver. Use this when the underlying asset is **not** managed by a Massdriver bundle but you want other components to be able to connect to it.

This resource is usable anywhere, not just inside a Massdriver bundle deployment. To declare a resource produced by a Massdriver bundle, use ` + "`massdriver_resource`" + ` instead.`,

		CreateContext: resourceImportedResourceCreate,
		ReadContext:   resourceImportedResourceRead,
		UpdateContext: resourceImportedResourceUpdate,
		DeleteContext: resourceImportedResourceDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Human-readable name for the resource.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"resource_type": {
				Description: "ID of the resource type this resource is an instance of (e.g., `aws-iam-role`). Immutable after creation.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"resource": {
				Description: "JSON-encoded resource data conforming to the resource type's schema.",
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Default:     "",
			},
		},
	}
}

func resourceImportedResourceCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	payload, err := decodeImportedResourcePayload(d.Get("resource").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	resource, createErr := api.CreateResource(ctx, client, d.Get("resource_type").(string), api.CreateResourceInput{
		Name:    d.Get("name").(string),
		Payload: payload,
	})
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(resource.ID)
	return resourceImportedResourceRead(ctx, d, meta)
}

func resourceImportedResourceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	resource, err := api.GetResource(ctx, client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", resource.Name)
	if resource.ResourceType != nil {
		d.Set("resource_type", resource.ResourceType.ID)
	}
	return nil
}

func resourceImportedResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	payload, err := decodeImportedResourcePayload(d.Get("resource").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	_, updateErr := api.UpdateResource(ctx, client, d.Id(), api.UpdateResourceInput{
		Name:    d.Get("name").(string),
		Payload: payload,
	})
	if updateErr != nil {
		return diag.FromErr(updateErr)
	}

	return resourceImportedResourceRead(ctx, d, meta)
}

func resourceImportedResourceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteResource(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

func decodeImportedResourcePayload(s string) (map[string]any, error) {
	if s == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(s), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON in resource: %w", err)
	}
	return payload, nil
}
