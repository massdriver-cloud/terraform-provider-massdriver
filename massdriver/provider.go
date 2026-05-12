package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
//
// v1.3 is a bridge release. The deprecated resources (`massdriver_artifact`,
// `massdriver_package_alarm`) remain fully functional so existing users keep
// working, while the replacement resources (`massdriver_resource`,
// `massdriver_instance_alarm`) ship alongside them as migration targets. v2.0
// removes the deprecated resources entirely.
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_artifact":       resourceArtifact(),
			"massdriver_package_alarm":  resourcePackageAlarm(),
			"massdriver_resource":       resourceResource(),
			"massdriver_instance_alarm": resourceInstanceAlarm(),
		},
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics
	client, clientErr := NewProviderClient()
	if clientErr != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Unable to create Massdriver client",
			Detail:   clientErr.Error(),
		})
		return nil, diags
	}

	return client, diags
}
