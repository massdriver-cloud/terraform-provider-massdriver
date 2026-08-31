package massdriver

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"api_key": {
				Description: "Massdriver API key — a service account token or a personal access token (`mds_` / `md_` prefix). Overrides `MASSDRIVER_API_KEY`. Required by every resource except `massdriver_resource` and `massdriver_instance_alarm`, which also accept the deployment token (`MASSDRIVER_DEPLOYMENT_ID` + `MASSDRIVER_TOKEN`) the platform injects into a bundle deployment. Setting both is supported — each resource uses the credential it needs, and the API key wins wherever both work. Keep this out of version control; prefer the env var or `TF_VAR_*` for production setups.",
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
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

// apiKeyResources are backed by GraphQL APIs that reject deployment tokens.
// New resources belong here: resourcesMap guards this map automatically, so
// nothing has to remember to opt in.
func apiKeyResources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_project":              resourceProject(),
		"massdriver_environment":          resourceEnvironment(),
		"massdriver_component":            resourceComponent(),
		"massdriver_component_link":       resourceComponentLink(),
		"massdriver_resource_grant":       resourceResourceGrant(),
		"massdriver_imported_resource":    resourceImportedResource(),
		"massdriver_group":                resourceGroup(),
		"massdriver_group_policy":         resourceGroupPolicy(),
		"massdriver_oci_repository":       resourceOciRepository(),
		"massdriver_oci_repository_grant": resourceOciRepositoryGrant(),
	}
}

// anyCredentialResources are GraphQL-backed, but the server accepts a
// deployment token for them too — bundles pair these with massdriver_resource
// and no API key.
func anyCredentialResources() map[string]*schema.Resource {
	return map[string]*schema.Resource{
		"massdriver_instance_alarm": resourceInstanceAlarm(),
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

// requireAuth wraps r's CRUD entry points with req's check. Import needs no
// wrapper: the custom importers only parse the composite ID, and terraform
// runs the wrapped Read immediately afterward.
func requireAuth(name string, r *schema.Resource, req authRequirement) {
	r.CreateContext = guardCRUD(name, r.CreateContext, req)
	r.ReadContext = guardCRUD(name, r.ReadContext, req)
	r.UpdateContext = guardCRUD(name, r.UpdateContext, req)
	r.DeleteContext = guardCRUD(name, r.DeleteContext, req)
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

	var diags diag.Diagnostics
	if os.Getenv(authDebugEnv) != "" {
		diags = append(diags, authDebugDiag(cfg, client))
	}
	return client, diags
}

// authDebugEnv, when non-empty, makes providerConfigure report which
// credential it resolved and where from.
const authDebugEnv = "MASSDRIVER_PROVIDER_DEBUG_AUTH"

// authDebugDiag describes the resolved credential without exposing it: only
// set/unset for secrets, plus the non-secret method, source, and ids.
func authDebugDiag(cfg ProviderConfig, pc *ProviderClient) diag.Diagnostic {
	var b strings.Builder

	fmt.Fprintf(&b, "provider block: api_key set=%t organization_id=%q url=%q\n",
		cfg.APIKey != "", cfg.OrganizationID, cfg.URL)

	b.WriteString("environment:\n")
	for _, name := range []string{
		"MASSDRIVER_API_KEY",
		"MASSDRIVER_ORGANIZATION_ID",
		"MASSDRIVER_ORG_ID",
		"MASSDRIVER_PROFILE",
		"MASSDRIVER_TOKEN",
		"MASSDRIVER_DEPLOYMENT_ID",
		"MASSDRIVER_URL",
	} {
		if v, ok := os.LookupEnv(name); ok && v != "" {
			fmt.Fprintf(&b, "  %s = set\n", name)
		}
	}

	configPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "massdriver", "config.yaml")
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "massdriver", "config.yaml")
	}
	_, statErr := os.Stat(configPath)
	fmt.Fprintf(&b, "config file: %s exists=%t\n", configPath, statErr == nil)

	creds := pc.Config.Credentials
	fmt.Fprintf(&b, "resolved: method=%q source=%q id=%q organization_id=%q\n",
		creds.Method, creds.Source, creds.ID, pc.Config.OrganizationID)

	errText := func(err error) string {
		if err == nil {
			return "<nil>"
		}
		return err.Error()
	}
	fmt.Fprintf(&b, "PlatformAuthErr: %s\nAlarmAuthErr: %s", errText(pc.PlatformAuthErr), errText(pc.AlarmAuthErr))

	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary:  "Massdriver provider credential debug",
		Detail:   b.String(),
	}
}
