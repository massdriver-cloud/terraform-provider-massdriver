package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_artifact":          resourceArtifact(),
			"massdriver_package_alarm":     resourcePackageAlarm(),
			"massdriver_project":           resourceProject(),
			"massdriver_environment":       resourceEnvironment(),
			"massdriver_component":         resourceComponent(),
			"massdriver_component_link":    resourceComponentLink(),
			"massdriver_resource":          resourceResource(),
			"massdriver_imported_resource": resourceImportedResource(),
			"massdriver_instance_alarm":    resourceInstanceAlarm(),
			"massdriver_group":             resourceGroup(),
			"massdriver_group_policy":      resourceGroupPolicy(),
			"massdriver_bundle_repository": resourceBundleRepository(),
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
