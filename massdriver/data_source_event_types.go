package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/eventdestinations"
)

// eventTypesAPI is the slice of *eventdestinations.Service this data source
// calls.
type eventTypesAPI interface {
	ListEventTypes(ctx context.Context, input eventdestinations.ListEventTypesInput) ([]eventdestinations.EventType, error)
}

var _ eventTypesAPI = (*eventdestinations.Service)(nil)

func dataSourceEventTypes() *schema.Resource {
	return &schema.Resource{
		Description: "The catalog of events that Massdriver records in the audit log. Use it to check the `event_types` of a `massdriver_event_destination`.",

		ReadContext: dataSourceEventTypesRead,

		Schema: map[string]*schema.Schema{
			"entity": {
				Description: "Only list the event types of this entity, such as `deployment`.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"ids": {
				Description: "The event type ids, sorted.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"event_types": {
				Description: "The event types, sorted by id.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Description: "Dotted, past-tense event id, such as `deployment.completed`.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"description": {
							Description: "When the event happens and what it holds.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"entity": {
							Description: "The entity the event applies to, such as `deployment`.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"action": {
							Description: "What happened to the entity, such as `completed`.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"change_type": {
							Description: "`CREATED`, `UPDATED`, or `DELETED`.",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func dataSourceEventTypesRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	entity := d.Get("entity").(string)
	eventTypes, err := pc.EventTypes.ListEventTypes(ctx, eventdestinations.ListEventTypesInput{Entity: entity})
	if err != nil {
		return diag.FromErr(err)
	}

	ids := make([]string, 0, len(eventTypes))
	flattened := make([]map[string]any, 0, len(eventTypes))
	for _, t := range eventTypes {
		ids = append(ids, t.ID)
		flattened = append(flattened, map[string]any{
			"id":          t.ID,
			"description": t.Description,
			"entity":      t.Entity,
			"action":      t.Action,
			"change_type": t.ChangeType,
		})
	}

	d.SetId("event_types/" + entity)
	d.Set("ids", ids)
	d.Set("event_types", flattened)
	return nil
}
