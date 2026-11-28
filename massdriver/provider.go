package massdriver

import (
	"context"
	"fmt"
	"maps"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_key": {
				Description: "Massdriver API key — a service account token or a personal access token (`mds_` / `md_` prefix). Overrides `MASSDRIVER_API_KEY`. Required by platform resources. Provisioning resources (`massdriver_resource`, `massdriver_instance_alarm`, `massdriver_stash_entry`) use the deployment token (`MASSDRIVER_DEPLOYMENT_ID` + `MASSDRIVER_TOKEN`) the platform injects into a bundle deployment. Setting both is supported — each resource uses the credential it needs, and the API key wins wherever both work. Keep this out of version control; prefer the env var or `TF_VAR_*` for production setups.",
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
		ResourcesMap:         resourcesMap(),
		DataSourcesMap:       dataSourcesMap(),
		ConfigureContextFunc: providerConfigure,
	}
}

// apiKeyResources are backed by GraphQL APIs that reject deployment tokens.
// New resources belong here: resourcesMap guards this map automatically, so
// nothing has to remember to opt in.
func apiKeyResources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_project":                          resourceProject(),
		"massdriver_environment":                      resourceEnvironment(),
		"massdriver_component":                        resourceComponent(),
		"massdriver_component_link":                   resourceComponentLink(),
		"massdriver_resource_grant":                   resourceResourceGrant(),
		"massdriver_imported_resource":                resourceImportedResource(),
		"massdriver_group":                            resourceGroup(),
		"massdriver_group_policy":                     resourceGroupPolicy(),
		"massdriver_oci_repository":                   resourceOciRepository(),
		"massdriver_oci_repository_grant":             resourceOciRepositoryGrant(),
		"massdriver_service_account":                  resourceServiceAccount(),
		"massdriver_service_account_group_membership": resourceServiceAccountGroupMembership(),
		"massdriver_custom_attribute":                 resourceCustomAttribute(),
		"massdriver_organization_settings":            resourceOrganizationSettings(),
		"massdriver_event_destination":                resourceEventDestination(),
	}
}

// apiKeyDataSources are read through GraphQL APIs that reject deployment
// tokens.
func apiKeyDataSources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_event_types": dataSourceEventTypes(),
	}
}

// anyCredentialDataSources accept an API key or a deployment token.
func anyCredentialDataSources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_stash_entry":   dataSourceStashEntry(),
		"massdriver_stash_entries": dataSourceStashEntries(),
	}
}

func dataSourcesMap() map[string]*schema.Resource {
	dataSources := map[string]*schema.Resource{}
	for name, r := range apiKeyDataSources() {
		requireAuth(name, r, requiresAPIKey)
		dataSources[name] = r
	}
	for name, r := range anyCredentialDataSources() {
		requireAuth(name, r, requiresAnyCredential)
		dataSources[name] = r
	}
	return dataSources
}

// anyCredentialResources are GraphQL-backed, but the server accepts a
// deployment token for them too — bundles pair these with massdriver_resource
// and no API key.
func anyCredentialResources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_instance_alarm": resourceInstanceAlarm(),
		"massdriver_stash_entry":    resourceStashEntry(),
	}
}

// deploymentTokenResources are backed by the deployment-token REST API. No
// guard: they resolve their own client through ProvisioningResources and
// report that error themselves.
func deploymentTokenResources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_resource": resourceResource(),
	}
}

// resourcesMap assembles the provider's resources, wrapping each with the
// credential check its API surface requires.
func resourcesMap() map[string]*schema.Resource {
	resources := map[string]*schema.Resource{}

	for name, r := range apiKeyResources() {
		requireAuth(name, r, requiresAPIKey)
		resources[name] = r
	}
	for name, r := range anyCredentialResources() {
		requireAuth(name, r, requiresAnyCredential)
		resources[name] = r
	}
	maps.Copy(resources, deploymentTokenResources())

	return resources
}

// authRequirement pairs a credential check with the error to show when it
// fails.
type authRequirement struct {
	err   func(*ProviderClient) error
	diags func(name string) diag.Diagnostics
}

var requiresAPIKey = authRequirement{
	err:   func(pc *ProviderClient) error { return pc.PlatformAuthErr },
	diags: apiKeyRequiredDiags,
}

var requiresAnyCredential = authRequirement{
	err:   func(pc *ProviderClient) error { return pc.AlarmAuthErr },
	diags: anyCredentialRequiredDiags,
}

// requireAuth wraps r's CRUD entry points and importer with req's check.
// Some importers call the API to resolve the ID, so they need the guard as
// much as Read does.
func requireAuth(name string, r *schema.Resource, req authRequirement) {
	r.CreateContext = guardCRUD(name, r.CreateContext, req)
	r.ReadContext = guardCRUD(name, r.ReadContext, req)
	r.UpdateContext = guardCRUD(name, r.UpdateContext, req)
	r.DeleteContext = guardCRUD(name, r.DeleteContext, req)
	if r.Importer != nil && r.Importer.StateContext != nil {
		r.Importer.StateContext = guardImport(name, r.Importer.StateContext, req)
	}
}

func guardImport(name string, next schema.StateContextFunc, req authRequirement) schema.StateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
		if pc, ok := meta.(*ProviderClient); ok && req.err(pc) != nil {
			authDiag := req.diags(name)[0]
			return nil, fmt.Errorf("%s: %s", authDiag.Summary, authDiag.Detail)
		}
		return next(ctx, d, meta)
	}
}

// crudFunc matches the four distinct-but-identical CRUD function types the
// terraform SDK declares, so one wrapper serves all of them.
type crudFunc interface {
	~func(context.Context, *schema.ResourceData, any) diag.Diagnostics
}

func guardCRUD[F crudFunc](name string, next F, req authRequirement) F {
	if next == nil {
		return nil
	}
	return F(func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		if pc, ok := meta.(*ProviderClient); ok && req.err(pc) != nil {
			return req.diags(name)
		}
		return next(ctx, d, meta)
	})
}

func apiKeyRequiredDiags(name string) diag.Diagnostics {
	return diag.Diagnostics{{
		Severity: diag.Error,
		Summary:  "No Massdriver API key configured",
		Detail: name + " is managed through the Massdriver platform API, which requires an API key or " +
			"personal access token. Set `api_key` in the provider block, or export MASSDRIVER_API_KEY " +
			"together with MASSDRIVER_ORGANIZATION_ID.",
	}}
}

func anyCredentialRequiredDiags(name string) diag.Diagnostics {
	return diag.Diagnostics{{
		Severity: diag.Error,
		Summary:  "No Massdriver credentials configured",
		Detail: name + " needs either an API key and organization ID (`api_key` in the provider block, " +
			"or MASSDRIVER_API_KEY with MASSDRIVER_ORGANIZATION_ID) or a deployment token " +
			"(MASSDRIVER_DEPLOYMENT_ID and MASSDRIVER_TOKEN, injected automatically inside a bundle " +
			"deployment). Neither was found.",
	}}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var cfg ProviderConfig
	if v, ok := d.GetOk("api_key"); ok {
		cfg.APIKey = v.(string)
	}
	if v, ok := d.GetOk("organization_id"); ok {
		cfg.OrganizationID = v.(string)
	}
	if v, ok := d.GetOk("url"); ok {
		cfg.URL = v.(string)
	}

	client, err := NewProviderClient(cfg)
	if err != nil {
		return nil, diag.Diagnostics{{
			Severity: diag.Error,
			Summary:  "Unable to create Massdriver client",
			Detail:   err.Error(),
		}}
	}
	return client, nil
}
