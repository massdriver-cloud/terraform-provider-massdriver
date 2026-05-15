package massdriver

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/components"
)

// componentIDSeparator joins a project identifier and a component identifier
// to form the platform component ID (e.g., `ecomm-db`).
const componentIDSeparator = "-"

// componentsAPI is the slice of *components.Service this resource calls.
type componentsAPI interface {
	Get(ctx context.Context, id string) (*components.Component, error)
	Add(ctx context.Context, projectID string, input components.AddInput) (*components.Component, error)
	Update(ctx context.Context, id string, input components.UpdateInput) (*components.Component, error)
	Remove(ctx context.Context, id string) (*components.Component, error)
}

var _ componentsAPI = (*components.Service)(nil)

func resourceComponent() *schema.Resource {
	return &schema.Resource{
		Description: "A component slot in a project's blueprint, backed by a bundle (OCI repository).",

		CreateContext: resourceComponentCreate,
		ReadContext:   resourceComponentRead,
		UpdateContext: resourceComponentUpdate,
		DeleteContext: resourceComponentDelete,

		Schema: map[string]*schema.Schema{
			"identifier": identifierSchema("component"),
			"project_id": {
				Description: "ID of the project this component belongs to.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"name": {
				Description: "Human-readable name for the component. Defaults to `identifier` if unset. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"bundle_name": {
				Description: "Name of the bundle (OCI repository) backing this component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"description": {
				Description: "Optional description of the component's purpose. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"attributes": attributesSchema("component"),
		},
	}
}

func resourceComponentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)
	projectID := d.Get("project_id").(string)

	identifier := d.Get("identifier").(string)
	name := d.Get("name").(string)
	if name == "" {
		name = identifier
	}

	component, err := pc.Components.Add(ctx, projectID, components.AddInput{
		OciRepoName: d.Get("bundle_name").(string),
		ID:          identifier,
		Name:        name,
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(component.ID)
	return resourceComponentRead(ctx, d, meta)
}

func resourceComponentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	component, err := pc.Components.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	projectID := ""
	if component.Project != nil {
		projectID = component.Project.ID
	} else if existing := d.Get("project_id").(string); existing != "" {
		// Carry through HCL-provided value when the API didn't return a Project
		// (shouldn't happen for Get, but defensive).
		projectID = existing
	} else if idx := strings.LastIndex(d.Id(), componentIDSeparator); idx > 0 {
		// Fall back to deriving from ID (mainly for `terraform import` paths).
		projectID = d.Id()[:idx]
	}

	d.Set("project_id", projectID)
	d.Set("name", component.Name)
	d.Set("description", component.Description)
	d.Set("attributes", attributesToState(component.Attributes))
	if component.OciRepo != nil {
		d.Set("bundle_name", component.OciRepo.Name)
	}

	prefix := projectID + componentIDSeparator
	if projectID != "" && strings.HasPrefix(component.ID, prefix) {
		d.Set("identifier", strings.TrimPrefix(component.ID, prefix))
	}

	return nil
}

func resourceComponentUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Components.Update(ctx, d.Id(), components.UpdateInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceComponentRead(ctx, d, meta)
}

func resourceComponentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Components.Remove(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
