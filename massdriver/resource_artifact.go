package massdriver

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceArtifact() *schema.Resource {
	return &schema.Resource{

		CreateContext: resourceArtifactCreate,
		ReadContext:   schema.NoopContext,
		UpdateContext: resourceArtifactUpdate,
		DeleteContext: resourceArtifactDelete,

		Schema: map[string]*schema.Schema{
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"artifact": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},
		},
	}
}

func resourceArtifactCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact := d.Get("artifact").(string)

	event := NewEvent(EVENT_TYPE_ARTIFACT_CREATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err := c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(time.Now().Format(time.RFC3339))
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact := d.Get("artifact").(string)

	event := NewEvent(EVENT_TYPE_ARTIFACT_UPDATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err := c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact := d.Get("artifact").(string)

	event := NewEvent(EVENT_TYPE_ARTIFACT_DELETED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err := c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}
