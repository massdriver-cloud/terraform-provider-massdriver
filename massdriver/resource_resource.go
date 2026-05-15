package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	provresources "github.com/massdriver-cloud/massdriver-sdk-go/massdriver/provisioning/resources"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

// provisioningResourcesAPI is the slice of *provisioning/resources.Service
// this resource calls. Lives here (not in client.go) so the interface is
// co-located with the code that uses it; the placeholder we keep in client.go
// is just a type-name reference that resolves to this declaration.
type provisioningResourcesAPI interface {
	CreateResource(ctx context.Context, a *provresources.Resource) (*provresources.Resource, error)
	GetResource(ctx context.Context, id string) (*provresources.Resource, error)
	UpdateResource(ctx context.Context, id string, a *provresources.Resource) (*provresources.Resource, error)
	DeleteResource(ctx context.Context, id, field string) error
}

var _ provisioningResourcesAPI = (*provresources.Service)(nil)

const (
	defaultResourceSchemaPath        = "../schema-artifacts.json"
	defaultResourceSpecificationPath = "../massdriver.yaml"
)

// resourceArtifactSchema is the shape of the schema-artifacts.json file. It
// contains JSON Schema fragments keyed by the resource's `field` name.
type resourceArtifactSchema struct {
	Properties map[string]any `json:"properties"`
}

// resourceBundleSpec is the shape of the relevant slice of massdriver.yaml.
// We only look at `artifacts.<field>.$ref` to derive the resource type.
type resourceBundleSpec struct {
	Artifacts struct {
		Properties map[string]map[string]string `yaml:"properties"`
	} `yaml:"artifacts"`
}

func resourceResource() *schema.Resource {
	return &schema.Resource{
		Description: `Declares a connectable resource produced by a Massdriver bundle. Use this **only** inside the IaC of a Massdriver bundle to satisfy a resource declared in the bundle's ` + "`massdriver.yaml`" + `; outside a deployment it will fail.

If you need to create a resource that is not managed by a Massdriver bundle, use ` + "`massdriver_imported_resource`" + ` instead.`,

		CreateContext: resourceResourceCreate,
		ReadContext:   resourceResourceRead,
		UpdateContext: resourceResourceUpdate,
		DeleteContext: resourceResourceDelete,

		Schema: map[string]*schema.Schema{
			"field": {
				Description: "The resource's `field` name as declared under `resources.properties` (formerly `artifacts.properties`) in the bundle's `massdriver.yaml`. Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"name": {
				Description: "Human-readable name for the resource.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"resource_type": {
				Description: "Resource type identifier (e.g., `aws-iam-role`). This attribute is computed from the `massdriver.yaml` specification.",
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
			},
			"resource": {
				Description: "JSON-encoded resource data. Validated locally against `schema-artifacts.json` (when present at `schema_path`) before being sent.",
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
			},
			"schema_path": {
				Description: "Path to the `schema-artifacts.json` JSON Schema file used for client-side validation. Defaults to `../schema-artifacts.json` (the location bundle scaffolding produces). Override only for local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     defaultResourceSchemaPath,
			},
			"specification_path": {
				Description: "Path to `massdriver.yaml`, used to look up the resource type from `$ref` when `resource_type` is unset. Defaults to `../massdriver.yaml`. Override only for local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     defaultResourceSpecificationPath,
			},
		},
	}
}

func resourceResourceCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	resource, err := buildResource(d, pc.Config.OrganizationID)
	if err != nil {
		return diag.FromErr(err)
	}

	created, err := api.CreateResource(ctx, resource)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(created.ID)
	d.Set("resource_type", resource.Type)
	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	got, err := api.GetResource(ctx, d.Id())
	if err != nil {
		if errors.Is(err, provresources.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("field", got.Field)
	d.Set("name", got.Name)
	d.Set("resource_type", got.Type)
	return nil
}

func resourceResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	resource, err := buildResource(d, pc.Config.OrganizationID)
	if err != nil {
		return diag.FromErr(err)
	}

	if _, err := api.UpdateResource(ctx, d.Id(), resource); err != nil {
		return diag.FromErr(err)
	}

	d.Set("resource_type", resource.Type)
	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	field := d.Get("field").(string)
	if err := api.DeleteResource(ctx, d.Id(), field); err != nil {
		// Already gone server-side — fine for destroy.
		if errors.Is(err, provresources.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// buildResource constructs the SDK Resource from terraform state, including
// schema validation, type lookup, and payload parsing.
func buildResource(d *schema.ResourceData, orgID string) (*provresources.Resource, error) {
	field := d.Get("field").(string)
	resourceJSON := d.Get("resource").(string)

	if err := validateResourceJSON(field, resourceJSON, d.Get("schema_path").(string)); err != nil {
		return nil, err
	}

	resourceType, err := resolveResourceType(d, orgID)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resourceJSON), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON in `resource`: %w", err)
	}

	return &provresources.Resource{
		Field:   field,
		Name:    d.Get("name").(string),
		Type:    resourceType,
		Payload: payload,
	}, nil
}

// validateResourceJSON runs the user's `resource` JSON against the JSON Schema
// extracted from schema-artifacts.json under `properties.<field>`.
func validateResourceJSON(field, resourceJSON, schemaPath string) error {
	if schemaPath == "" {
		schemaPath = defaultResourceSchemaPath
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("unable to open schema file: %s", schemaPath)
	}

	var schemaObj resourceArtifactSchema
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		return fmt.Errorf("invalid JSON in %s: %w", schemaPath, err)
	}

	specificSchema, exists := schemaObj.Properties[field]
	if !exists {
		return fmt.Errorf(`resource validation failed: field %q does not exist in schema`, field)
	}

	sl := gojsonschema.NewGoLoader(specificSchema.(map[string]any))
	dl := gojsonschema.NewStringLoader(resourceJSON)

	result, err := gojsonschema.Validate(sl, dl)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return errors.New("resource validation failed: " + result.Errors()[0].String())
	}
	return nil
}

// resolveResourceType returns the resource type to send to the API.
//
// If `resource_type` is set in state (either explicitly by the user or
// computed from a previous apply) we use it verbatim. Otherwise — only on
// the first apply, before the field has been computed — we fall back to
// reading `artifacts.<field>.$ref` from massdriver.yaml. Bare type IDs (no
// slash) are prefixed with the org ID, matching the legacy artifact behavior.
func resolveResourceType(d *schema.ResourceData, orgID string) (string, error) {
	if existing := d.Get("resource_type").(string); existing != "" {
		return prefixOrgIfNeeded(existing, orgID), nil
	}

	field := d.Get("field").(string)
	specPath := d.Get("specification_path").(string)
	if specPath == "" {
		specPath = defaultResourceSpecificationPath
	}

	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("unable to open specification file: %s", specPath)
	}

	var spec resourceBundleSpec
	if err := yaml.Unmarshal(specBytes, &spec); err != nil {
		return "", fmt.Errorf("invalid YAML in %s: %w", specPath, err)
	}

	artifactSpec, exists := spec.Artifacts.Properties[field]
	if !exists {
		return "", fmt.Errorf(`field %q does not exist in %s`, field, specPath)
	}

	ref, exists := artifactSpec["$ref"]
	if !exists {
		return "", fmt.Errorf(`field %q in %s has no $ref`, field, specPath)
	}

	return prefixOrgIfNeeded(ref, orgID), nil
}

// prefixOrgIfNeeded: a bare type ID like `aws-iam-role` becomes
// `<orgID>/aws-iam-role`; a fully-qualified type with a slash is left alone.
func prefixOrgIfNeeded(typeRef, orgID string) string {
	if strings.Contains(typeRef, "/") {
		return typeRef
	}
	return orgID + "/" + typeRef
}
