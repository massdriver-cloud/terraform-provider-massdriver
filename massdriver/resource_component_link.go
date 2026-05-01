package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

func resourceComponentLink() *schema.Resource {
	return &schema.Resource{
		Description: "A design-time link between two components in a project's blueprint, wiring an output of one component to an input of another.",

		CreateContext: resourceComponentLinkCreate,
		ReadContext:   resourceComponentLinkRead,
		DeleteContext: resourceComponentLinkDelete,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Description: "ID of the project that owns both components.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"from_component_id": {
				Description: "ID of the component producing the resource (source of the link).",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"from_field": {
				Description: "Output field name on the source component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"from_version": {
				Description: "Version constraint for the source component (e.g., '~1.0', '1.2.3', 'latest').",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"to_component_id": {
				Description: "ID of the component consuming the resource (destination of the link).",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"to_field": {
				Description: "Input field name on the destination component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"to_version": {
				Description: "Version constraint for the destination component (e.g., '~1.0', '1.2.3', 'latest').",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceComponentLinkCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	link, err := api.LinkComponents(ctx, client, api.LinkComponentsInput{
		FromComponentId: d.Get("from_component_id").(string),
		FromField:       d.Get("from_field").(string),
		FromVersion:     d.Get("from_version").(string),
		ToComponentId:   d.Get("to_component_id").(string),
		ToField:         d.Get("to_field").(string),
		ToVersion:       d.Get("to_version").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(link.ID)
	return resourceComponentLinkRead(ctx, d, meta)
}

func resourceComponentLinkRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	projectID := d.Get("project_id").(string)
	fromComponentID := d.Get("from_component_id").(string)
	toComponentID := d.Get("to_component_id").(string)

	filter := &api.LinksFilter{
		FromComponentId: &api.IdFilter{Eq: fromComponentID},
		ToComponentId:   &api.IdFilter{Eq: toComponentID},
	}
	links, err := api.ListLinks(ctx, client, projectID, filter)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, link := range links {
		if link.ID == d.Id() {
			d.Set("from_field", link.FromField)
			d.Set("to_field", link.ToField)
			if link.FromComponent != nil {
				d.Set("from_component_id", link.FromComponent.ID)
			}
			if link.ToComponent != nil {
				d.Set("to_component_id", link.ToComponent.ID)
			}
			return nil
		}
	}

	d.SetId("")
	return nil
}

func resourceComponentLinkDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.UnlinkComponents(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
