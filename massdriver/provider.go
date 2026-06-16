package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_key": {
				Description: "Massdriver API key — a service account token or a personal access token (`mds_` / `md_` prefix). Overrides `MASSDRIVER_API_KEY`. Keep this out of version control; prefer the env var or `TF_VAR_*` for production setups.",
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
			},
			"organization_id": {
				Description: "Massdriver organization ID this provider operates against. Overrides `MASSDRIVER_ORGANIZATION_ID`.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"url": {
				Description: "Massdriver API base URL. Overrides `MASSDRIVER_URL`. Only set when targeting a self-hosted instance.",
				Type:        schema.TypeString,
				Optional:    true,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"massdriver_project":           resourceProject(),
			"massdriver_environment":       resourceEnvironment(),
			"massdriver_component":         resourceComponent(),
			"massdriver_component_link":    resourceComponentLink(),
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
	var opts []massdriver.Option
	if v, ok := d.GetOk("api_key"); ok {
		opts = append(opts, massdriver.WithAPIKey(v.(string)))
	}
	if v, ok := d.GetOk("organization_id"); ok {
		opts = append(opts, massdriver.WithOrganizationID(v.(string)))
	}
	if v, ok := d.GetOk("url"); ok {
		opts = append(opts, massdriver.WithBaseURL(v.(string)))
	}

	client, err := NewProviderClient(opts...)
	if err != nil {
		return nil, diag.Diagnostics{{
			Severity: diag.Error,
			Summary:  "Unable to create Massdriver client",
			Detail:   err.Error(),
		}}
	}
	return client, nil
}
