package massdriver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

func resourceResource() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver resource (replaces the v0 artifact concept). A resource is an instance of a resource type — cloud credentials, a connection string, or any other connectable output.",

		CreateContext: resourceResourceCreate,
		ReadContext:   resourceResourceRead,
		UpdateContext: resourceResourceUpdate,
		DeleteContext: resourceResourceDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Human-readable name for the resource.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"resource_type_id": {
				Description: "ID of the resource type this resource is an instance of (e.g., 'aws-iam-role'). Immutable after creation.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"payload": {
				Description: "JSON-encoded resource payload conforming to the resource type's schema.",
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Default:     "",
			},
		},
	}
}

func resourceResourceCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	payload, err := decodeResourcePayload(d.Get("payload").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	resource, createErr := api.CreateResource(ctx, client, d.Get("resource_type_id").(string), api.CreateResourceInput{
		Name:    d.Get("name").(string),
		Payload: payload,
	})
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(resource.ID)
	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	resource, err := api.GetResource(ctx, client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", resource.Name)
	if resource.ResourceType != nil {
		d.Set("resource_type_id", resource.ResourceType.ID)
	}
	return nil
}

func resourceResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	payload, err := decodeResourcePayload(d.Get("payload").(string))
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

	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteResource(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

func decodeResourcePayload(s string) (map[string]any, error) {
	if s == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(s), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON in payload: %w", err)
	}
	return payload, nil
}
