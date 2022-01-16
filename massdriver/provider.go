package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"deployment_id": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_DEPLOYMENT_ID", nil),
			},
			"token": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_TOKEN", nil),
			},
			"event_topic_arn": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_EVENT_TOPIC_ARN", nil),
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_artifact": resourceArtifact(),
		},
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	deployment_id := d.Get("deployment_id").(string)
	token := d.Get("token").(string)
	eventTopicARN := d.Get("event_topic_arn").(string)

	var diags diag.Diagnostics
	c, err := NewMassdriverClient(deployment_id, token, eventTopicARN)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Unable to create Massdriver client",
			Detail:   err.Error(),
		})
		return nil, diags
	}

	return c, diags
}
