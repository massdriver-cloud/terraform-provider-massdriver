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
				Description: "Deployment ID, to be used in automation for linking resources back to a Massdriver deployment. This field is only used in automation.",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_DEPLOYMENT_ID", nil),
			},
			"token": {
				Description: "Deployment token, for authenticating to Massdriver. This field is only used in automation.",
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_TOKEN", nil),
			},
			"event_topic_arn": {
				Description: "ARN of SNS topic to publish events to. This field is only used in automation.",
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_EVENT_TOPIC_ARN", nil),
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_artifact":      resourceArtifact(),
			"massdriver_package_alarm": resourcePackageAlarm(),
		},
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	deploymentId := d.Get("deployment_id").(string)
	token := d.Get("token").(string)
	eventTopicARN := d.Get("event_topic_arn").(string)

	var diags diag.Diagnostics
	c, err := NewMassdriverClient(deploymentId, token, eventTopicARN)
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
