package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	provresources "github.com/massdriver-cloud/massdriver-sdk-go/massdriver/provisioning/resources"
)

// provisioningResourcesAPI is the slice of *provisioning/resources.Service
// this resource calls. Lives here (not in client.go) so the interface is
// co-located with the code that uses it; the placeholder we keep in client.go
// is just a type-name reference that resolves to this declaration.
type provisioningResourcesAPI interface {
	CreateResource(ctx context.Context, input *provresources.ResourceInput) (*provresources.Resource, error)
	GetResource(ctx context.Context, id string) (*provresources.Resource, error)
	UpdateResource(ctx context.Context, id string, input *provresources.ResourceInput) (*provresources.Resource, error)
	DeleteResource(ctx context.Context, id string) error
}

var _ provisioningResourcesAPI = (*provresources.Service)(nil)

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
			"resource": {
				Description: "JSON-encoded resource data.",
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
			},
			"resource_type": {
				Description: "Resolved resource type in `identifier@version` form (e.g. `aws-iam-role@1.2.3`), resolved server-side from the deployment's release pin and `field`.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"specification_path": {
				Description: "Deprecated and ignored. The resource type is resolved server-side, so the bundle's `massdriver.yaml` is no longer read.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Deprecated:  "specification_path is ignored: the resource type is resolved server-side from the deployment's release pin and `field`. Remove it from your configuration; the argument will be deleted in the next major version.",
			},
			"available_upgrade": {
				Description: "The newest published version within the bundle's declared version range that is newer than the one `resource_type` names (e.g. `1.3.0`), empty when there is none. A non-empty value makes the next plan an in-place update onto that version.",
				Type:        schema.TypeString,
				Computed:    true,
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

	input, err := buildResourceInput(d)
	if err != nil {
		return diag.FromErr(err)
	}

	created, err := api.CreateResource(ctx, input)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(created.ID)
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

	resourceType := got.ResourceType
	if resourceType == "" {
		// Compatibility shim: self-hosted APIs older than this change don't
		// return resource_type. Fall back to the legacy `type`, minus its org
		// prefix. Remove once self-hosted deployments have caught up.
		resourceType = got.Type[strings.LastIndex(got.Type, "/")+1:]
	}

	d.Set("field", got.Field)
	d.Set("name", got.Name)
	d.Set("resource_type", resourceType)
	d.Set("available_upgrade", got.AvailableUpgrade)
	return nil
}

func resourceResourceUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	api, err := pc.ProvisioningResources()
	if err != nil {
		return diag.FromErr(err)
	}

	input, err := buildResourceInput(d)
	if err != nil {
		return diag.FromErr(err)
	}

	if _, err := api.UpdateResource(ctx, d.Id(), input); err != nil {
		return diag.FromErr(err)
	}

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

// resourceResourceCustomizeDiff plans an in-place update when the API reports
// a newer version in range. A version constraint (`~1`) is resolved to a
// concrete version only when the resource is written, so the write the update
// performs is what moves the resource onto it.
func resourceResourceCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	upgrade := d.Get("available_upgrade").(string)
	if d.Id() == "" || upgrade == "" {
		return nil
	}

	// Should a newer release land between plan and apply, the server resolves
	// to that one and the applied value differs from the planned one.
	// helper/schema marks this provider UnsafeToUseLegacyTypeSystem, so
	// terraform logs the mismatch rather than failing the apply.
	identifier, _, _ := strings.Cut(d.Get("resource_type").(string), "@")
	if err := d.SetNew("resource_type", identifier+"@"+upgrade); err != nil {
		return err
	}
	// Nothing stays pending afterwards, but helper/schema normalizes a computed
	// attribute planned as "" to unknown, so plan it that way outright.
	return d.SetNewComputed("available_upgrade")
}

// buildResourceInput constructs the create/update body. No resource type is
// sent: the server resolves it from the deployment's release pin and `field`.
func buildResourceInput(d *schema.ResourceData) (*provresources.ResourceInput, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(d.Get("resource").(string)), &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON in `resource`: %w", err)
	}

	return &provresources.ResourceInput{
		Field:   d.Get("field").(string),
		Name:    d.Get("name").(string),
		Payload: payload,
	}, nil
}
