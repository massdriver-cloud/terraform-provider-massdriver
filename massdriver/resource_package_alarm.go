package massdriver

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type PackageAlarmMetadata struct {
	ResourceIdentifier string `json:"cloud_resource_id"`
	DisplayName        string `json:"display_name"`
}

func resourcePackageAlarm() *schema.Resource {
	return &schema.Resource{
		Description: "This resource registers a package alarm in the Massdriver console for presentation to the user",

		CreateContext: resourcePackageAlarmCreate,
		ReadContext:   schema.NoopContext,
		UpdateContext: resourcePackageAlarmUpdate,
		DeleteContext: resourcePackageAlarmDelete,

		Schema: map[string]*schema.Schema{
			"cloud_resource_id": {
				Description: "The identifier of the alarm. In Azure it will be the id, GCP will be the name, and in AWS it will be the arn",
				Type:        schema.TypeString,
				Required:    true,
			},
			"display_name": {
				Description: "The name to display in the massdriver UI",
				Type:        schema.TypeString,
				Required:    true,
			},
			"last_updated": {
				Description: "A timestamp of when the last time this resource was updated",
				Type:        schema.TypeString,
				Optional:    false,
				Required:    false,
				Computed:    true,
			},
		},
	}
}

func resourcePackageAlarmCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	packageAlarmMeta := PackageAlarmMetadata{
		ResourceIdentifier: d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
	}

	event := NewEvent(EVENT_TYPE_ALARM_CHANNEL_CREATED)
	event.Payload = EventPayloadAlarmChannels{DeploymentId: c.DeploymentID, PackageAlarm: packageAlarmMeta}

	err := c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId(time.Now().Format(time.RFC3339))
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	packageAlarmMeta := PackageAlarmMetadata{
		ResourceIdentifier: d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
	}

	event := NewEvent(EVENT_TYPE_ALARM_CHANNEL_UPDATED)
	event.Payload = EventPayloadAlarmChannels{DeploymentId: c.DeploymentID, PackageAlarm: packageAlarmMeta}

	err := c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	packageAlarmMeta := PackageAlarmMetadata{
		ResourceIdentifier: d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
	}

	event := NewEvent(EVENT_TYPE_ALARM_CHANNEL_DELETED)
	event.Payload = EventPayloadAlarmChannels{DeploymentId: c.DeploymentID, PackageAlarm: packageAlarmMeta}

	err := c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId("")

	return diags
}
