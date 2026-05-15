package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/resources"
)

// resourcesAPI is the slice of *platform/resources.Service this resource calls.
type resourcesAPI interface {
	Get(ctx context.Context, id string) (*resources.Resource, error)
	Create(ctx context.Context, resourceTypeID string, input resources.CreateInput) (*resources.Resource, error)
	Update(ctx context.Context, id string, input resources.UpdateInput) (*resources.Resource, error)
	Delete(ctx context.Context, id string) (*resources.Resource, error)
}

var _ resourcesAPI = (*resources.Service)(nil)

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
	pc := meta.(*ProviderClient)

	payload, err := decodeImportedResourcePayload(d.Get("resource").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	resource, err := pc.Resources.Create(ctx, d.Get("resource_type").(string), resources.CreateInput{
		Name:    d.Get("name").(string),
		Payload: payload,
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(resource.ID)
	return resourceImportedResourceRead(ctx, d, meta)
}

func resourceImportedResourceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	resource, err := pc.Resources.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("name", resource.Name)
	if resource.ResourceType != nil {
		d.Set("resource_type", resource.ResourceType.ID)
	}
	return nil
}

func resourceImportedResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	payload, err := decodeImportedResourcePayload(d.Get("resource").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	if _, err := pc.Resources.Update(ctx, d.Id(), resources.UpdateInput{
		Name:    d.Get("name").(string),
		Payload: payload,
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceImportedResourceRead(ctx, d, meta)
}

func resourceImportedResourceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Resources.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
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
