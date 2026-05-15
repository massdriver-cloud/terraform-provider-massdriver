package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider is the v2.0 provider. The deprecated v1 resources
// (`massdriver_artifact`, `massdriver_package_alarm`) have been removed; users
// on `~> 1.0` must migrate to `massdriver_resource` and
// `massdriver_instance_alarm` before upgrading.
//
// `massdriver_component_link` is intentionally not exposed yet — it will land
// in a follow-up release once the bundle-version semantics it relies on are
// settled.
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_project":           resourceProject(),
			"massdriver_environment":       resourceEnvironment(),
			"massdriver_component":         resourceComponent(),
			"massdriver_resource":          resourceResource(),
			"massdriver_imported_resource": resourceImportedResource(),
			"massdriver_instance_alarm":    resourceInstanceAlarm(),
			"massdriver_group":             resourceGroup(),
			"massdriver_group_policy":      resourceGroupPolicy(),
			"massdriver_oci_repository":    resourceOciRepository(),
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
