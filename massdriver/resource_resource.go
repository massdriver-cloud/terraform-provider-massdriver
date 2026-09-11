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
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/resources"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

const (
	defaultResourceSchemaPath        = "../schema-artifacts.json"
	defaultResourceSpecificationPath = "../massdriver.yaml"
)

// resourceArtifactSchema is the shape of the schema-artifacts.json file. It
// contains JSON Schema fragments keyed by the resource's `field` name.
type resourceArtifactSchema struct {
	Properties map[string]any `json:"properties"`
}

// resourcesBlock is an entry in the modern `resources` map of massdriver.yaml:
//
//	resources:
//	  my_field:
//	    resource_type: my-org/my-type
//	    required: true
type resourcesBlock struct {
	ResourceType string `yaml:"resource_type"`
	Required     bool   `yaml:"required"`
}

// resourceBundleSpec is the shape of the relevant slice of massdriver.yaml.
// The resource type comes from `resources.<field>.resource_type`, falling back
// to the legacy `artifacts.properties.<field>.$ref`:
//
//	artifacts:
//	  properties:
//	    my_field:
//	      $ref: my-type
type resourceBundleSpec struct {
	Resources map[string]resourcesBlock `yaml:"resources"`
	Artifacts struct {
		Properties map[string]map[string]string `yaml:"properties"`
	} `yaml:"artifacts"`
}

func resourceResource() *schema.Resource {
	return &schema.Resource{
		Description: `Creates a provisioned resource produced by a Massdriver bundle. Use this **only** inside the IaC of a Massdriver bundle to satisfy a resource declared in the bundle's ` + "`massdriver.yaml`" + `; outside a deployment it will fail. Replaces the deprecated ` + "`massdriver_artifact`" + ` resource.`,

		CreateContext: resourceResourceCreate,
		ReadContext:   resourceResourceRead,
		UpdateContext: resourceResourceUpdate,
		DeleteContext: resourceResourceDelete,

		Schema: map[string]*schema.Schema{
			"field": {
				Description: "The resource's `field` name as declared in the bundle's `massdriver.yaml` — either as a key under `resources` or, in the legacy layout, under `artifacts.properties`. Immutable.",
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
				Description: "JSON-encoded resource data. Validated locally against `schema-artifacts.json` when that file is present at `schema_path`; otherwise validated server-side.",
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
			},
			"schema_path": {
				Description: "Path to the `schema-artifacts.json` JSON Schema file used for client-side validation. Defaults to `../schema-artifacts.json`. The Massdriver CLI no longer emits this file; when it is absent, client-side validation is skipped and the resource is validated server-side instead. Override only for local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     defaultResourceSchemaPath,
			},
			"specification_path": {
				Description: "Path to `massdriver.yaml`, used to look up the resource type when `resource_type` is unset. Reads `resources.<field>.resource_type`, falling back to the legacy `artifacts.properties.<field>.$ref`. Defaults to `../massdriver.yaml`. Override only for local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     defaultResourceSpecificationPath,
			},
		},
	}
}

func resourceResourceCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := requireDeploymentAuth(pc.Client); err != nil {
		return diag.FromErr(err)
	}

	resource, err := buildResource(d, pc.Client)
	if err != nil {
		return diag.FromErr(err)
	}

	created, createErr := pc.ResourceService().CreateResource(ctx, resource)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(created.ID)
	d.Set("resource_type", resource.Type)
	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := requireDeploymentAuth(pc.Client); err != nil {
		return diag.FromErr(err)
	}

	got, err := pc.ResourceService().GetResource(ctx, d.Id())
	if err != nil {
		if err.Error() == "not found" {
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

	if err := requireDeploymentAuth(pc.Client); err != nil {
		return diag.FromErr(err)
	}

	resource, err := buildResource(d, pc.Client)
	if err != nil {
		return diag.FromErr(err)
	}

	if _, updateErr := pc.ResourceService().UpdateResource(ctx, d.Id(), resource); updateErr != nil {
		return diag.FromErr(updateErr)
	}

	d.Set("resource_type", resource.Type)
	return resourceResourceRead(ctx, d, meta)
}

func resourceResourceDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := requireDeploymentAuth(pc.Client); err != nil {
		return diag.FromErr(err)
	}

	field := d.Get("field").(string)
	if err := pc.ResourceService().DeleteResource(ctx, d.Id(), field); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// requireDeploymentAuth fast-fails with a clear error when the caller isn't
// running inside a Massdriver deployment. The endpoint backing massdriver_resource
// only accepts deployment-scoped credentials — checking up front gives a
// clearer message than the opaque 401 we'd get from the server.
func requireDeploymentAuth(mdClient *client.Client) error {
	if mdClient == nil || mdClient.Config.Credentials == nil || mdClient.Config.Credentials.Method != config.AuthDeployment {
		return fmt.Errorf("massdriver_resource can only be used inside a Massdriver bundle deployment (MASSDRIVER_DEPLOYMENT_TOKEN must be set)")
	}
	return nil
}

// buildResource constructs the SDK Resource from terraform state, including
// schema validation, type lookup, and payload parsing.
func buildResource(d *schema.ResourceData, mdClient *client.Client) (*resources.Resource, error) {
	field := d.Get("field").(string)
	resourceJSON := d.Get("resource").(string)

	if err := validateResourceJSON(field, resourceJSON, d.Get("schema_path").(string)); err != nil {
		return nil, err
	}

	resourceType, err := resolveResourceType(d, mdClient)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resourceJSON), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON in `resource`: %w", err)
	}

	return &resources.Resource{
		Field:   field,
		Name:    d.Get("name").(string),
		Type:    resourceType,
		Payload: payload,
	}, nil
}

// validateResourceJSON runs the user's `resource` JSON against the JSON Schema
// extracted from schema-artifacts.json under `properties.<field>`, when that
// schema is available. Mirrors the behavior of the deprecated
// `massdriver_artifact` resource.
//
// The Massdriver CLI no longer emits schema-artifacts.json, and resources are
// validated server-side, so an unavailable schema is not an error: we skip
// client-side validation and let the API be the authority. Only a schema we
// can actually load and resolve for this field is enforced.
func validateResourceJSON(field, resourceJSON, schemaPath string) error {
	if schemaPath == "" {
		schemaPath = defaultResourceSchemaPath
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		// No schema file (the common case since the CLI stopped emitting it).
		return nil
	}

	var schemaObj resourceArtifactSchema
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		// Unreadable schema file; fall back to server-side validation.
		return nil
	}

	specificSchema, exists := schemaObj.Properties[field]
	if !exists {
		// No schema for this field; fall back to server-side validation.
		return nil
	}
	schemaMap, ok := specificSchema.(map[string]any)
	if !ok {
		// Not an object schema (e.g. a boolean); nothing we can enforce here.
		return nil
	}

	sl := gojsonschema.NewGoLoader(schemaMap)
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
// the first apply, before the field has been computed — we read it from
// massdriver.yaml: `resources.<field>.resource_type` first, falling back to
// the legacy `artifacts.properties.<field>.$ref`. Bare type IDs (no slash)
// are prefixed with the org ID, matching the legacy artifact behavior.
func resolveResourceType(d *schema.ResourceData, mdClient *client.Client) (string, error) {
	if existing := d.Get("resource_type").(string); existing != "" {
		return prefixOrgIfNeeded(existing, mdClient.Config.OrganizationID), nil
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

	// Modern `resources` block takes precedence.
	if resourceSpec, exists := spec.Resources[field]; exists {
		if resourceSpec.ResourceType == "" {
			return "", fmt.Errorf(`field %q in %s has empty resource_type`, field, specPath)
		}
		return prefixOrgIfNeeded(resourceSpec.ResourceType, mdClient.Config.OrganizationID), nil
	}

	// Legacy `artifacts.properties` block.
	if artifactSpec, exists := spec.Artifacts.Properties[field]; exists {
		ref, hasRef := artifactSpec["$ref"]
		if !hasRef || ref == "" {
			return "", fmt.Errorf(`field %q in %s has no $ref`, field, specPath)
		}
		return prefixOrgIfNeeded(ref, mdClient.Config.OrganizationID), nil
	}

	return "", fmt.Errorf(`field %q not found in "resources" or "artifacts" of %s`, field, specPath)
}

// prefixOrgIfNeeded matches the legacy artifact behavior: a bare type ID like
// `aws-iam-role` becomes `<orgID>/aws-iam-role`; a fully-qualified type with a
// slash is left alone.
func prefixOrgIfNeeded(typeRef, orgID string) string {
	if strings.Contains(typeRef, "/") {
		return typeRef
	}
	return orgID + "/" + typeRef
}
