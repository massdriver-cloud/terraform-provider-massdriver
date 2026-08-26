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
	DeleteResource(ctx context.Context, id string) error
}

var _ provisioningResourcesAPI = (*provresources.Service)(nil)

const (
	defaultResourceSpecificationPath = "../massdriver.yaml"
)

// resourceBundleSpec is the shape of the relevant slice of massdriver.yaml.
// The resource type comes from `resources.<field>.resource_type`, falling
// back to the legacy `artifacts.properties.<field>.$ref`.
type resourcesBlock struct {
	ResourceType string `yaml:"resource_type"`
	Required     bool   `yaml:"required"`
}
type artifactPropertyBlock struct {
	Ref string `yaml:"$ref"`
}
type resourceBundleSpec struct {
	Resources map[string]resourcesBlock `yaml:"resources"`
	Artifacts struct {
		Properties map[string]artifactPropertyBlock `yaml:"properties"`
	} `yaml:"artifacts"`
}

func resourceResource() *schema.Resource {
	return &schema.Resource{
		Description: `Creates a provisioned resource produced by a Massdriver bundle. Use this **only** inside the IaC of a Massdriver bundle to satisfy a resource declared in the bundle's ` + "`massdriver.yaml`" + `; outside a deployment it will fail.

If you need to create a resource that is not managed by a Massdriver bundle, use ` + "`massdriver_imported_resource`" + ` instead.`,

		CreateContext: resourceResourceCreate,
		ReadContext:   resourceResourceRead,
		UpdateContext: resourceResourceUpdate,
		DeleteContext: resourceResourceDelete,
		CustomizeDiff: resourceResourceCustomizeDiff,

		Schema: map[string]*schema.Schema{
			"field": {
				Description: "The resource's `field` name as declared under `resources` (formerly `artifacts.properties`) in the bundle's `massdriver.yaml`. Immutable.",
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
				Description: "Resource type identifier (e.g., `aws-iam-role`). Computed at plan time from the bundle's `massdriver.yaml`; when it changes there (e.g., a version bump), the resource is replaced.",
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
			},
			"resource": {
				Description: "JSON-encoded resource data.",
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
			},
			"specification_path": {
				Description: "Path to `massdriver.yaml`, used to look up the resource type. Defaults to `../massdriver.yaml`. Override only for local provider testing.",
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

	resource, err := buildResource(d)
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
	resourceType := stripOrgPrefix(got.Type)

	d.Set("field", got.Field)
	d.Set("name", got.Name)
	d.Set("resource_type", resourceType)
	return nil
}

func resourceResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	resource, err := buildResource(d)
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

	if err := api.DeleteResource(ctx, d.Id()); err != nil {
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

// resourceResourceCustomizeDiff resolves the resource type from
// massdriver.yaml at plan time so a change there (e.g. a version bump on a
// versioned resource type) produces a diff — and, since `resource_type` is
// ForceNew, a replacement — even when nothing in the terraform config changed.
func resourceResourceCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	// If the inputs aren't known yet (interpolated from another resource's
	// unknown output), leave resource_type to be resolved at apply time.
	if !d.NewValueKnown("field") || !d.NewValueKnown("specification_path") {
		return nil
	}

	resourceType, err := resolveResourceType(d.Get("field").(string), d.Get("specification_path").(string))
	if err != nil {
		return err
	}
	if resourceType != d.Get("resource_type").(string) {
		return d.SetNew("resource_type", resourceType)
	}
	return nil
}

// buildResource constructs the SDK Resource from terraform state, including
// type lookup and payload parsing.
func buildResource(d *schema.ResourceData) (*provresources.Resource, error) {
	field := d.Get("field").(string)
	resourceJSON := d.Get("resource").(string)

	// CustomizeDiff resolves resource_type at plan time, so normally we just
	// send the planned value. It's only empty when plan-time inputs were
	// unknown and the resolution was deferred; resolve it now.
	resourceType := d.Get("resource_type").(string)
	if resourceType == "" {
		var err error
		resourceType, err = resolveResourceType(field, d.Get("specification_path").(string))
		if err != nil {
			return nil, err
		}
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

// resolveResourceType looks up the resource type for `field` in the bundle's
// massdriver.yaml: `resources.<field>.resource_type` first, falling back to
// the legacy `artifacts.properties.<field>.$ref`. Types are canonically bare
// in v2, so any org qualifier is stripped before use.
func resolveResourceType(field, specPath string) (string, error) {
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

	resourceSpec, resourceSpecExists := spec.Resources[field]
	if resourceSpecExists {
		if resourceSpec.ResourceType == "" {
			return "", fmt.Errorf(`field %q in %s has empty resource_type`, field, specPath)
		}
		return stripOrgPrefix(resourceSpec.ResourceType), nil
	}

	artifactSpec, artifactSpecExists := spec.Artifacts.Properties[field]
	if artifactSpecExists {
		if artifactSpec.Ref == "" {
			return "", fmt.Errorf(`field %q in %s has empty $ref`, field, specPath)
		}
		return stripOrgPrefix(artifactSpec.Ref), nil
	}

	return "", fmt.Errorf(`field %q not found in "resources" or "artifacts" of %s`, field, specPath)
}

// stripOrgPrefix removes the org prefix from a resource type, if present.
func stripOrgPrefix(resourceType string) string {
	parts := strings.Split(resourceType, "/")
	return parts[len(parts)-1]
}
