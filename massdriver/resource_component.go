package massdriver

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

// componentIDSeparator joins a project identifier and a component identifier
// to form the platform component ID (e.g., `ecomm*db`).
const componentIDSeparator = "*"

func resourceComponent() *schema.Resource {
	return &schema.Resource{
		Description: "A component slot in a project's blueprint, backed by a bundle (OCI repository).",

		CreateContext: resourceComponentCreate,
		ReadContext:   resourceComponentRead,
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
				Description: "Human-readable name for the component. Defaults to `identifier` if unset. Changing it forces replacement (the API has no component-update mutation). When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
			},
			"oci_repo_name": {
				Description: "Name of the OCI repository (bundle) backing this component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"description": {
				Description: "Optional description of the component's purpose. Changing it forces replacement. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				ForceNew:    true,
			},
		},
	}
}

func resourceComponentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client
	projectID := d.Get("project_id").(string)

	identifier := d.Get("identifier").(string)
	name := d.Get("name").(string)
	if name == "" {
		name = identifier
	}

	component, err := api.AddComponent(ctx, client, projectID, d.Get("oci_repo_name").(string), api.AddComponentInput{
		Id:          identifier,
		Name:        name,
		Description: d.Get("description").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(component.ID)
	return resourceComponentRead(ctx, d, meta)
}

func resourceComponentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	projectID := d.Get("project_id").(string)
	if projectID == "" {
		// Recover project_id from the component ID during `terraform import`.
		// Component IDs follow `<project>*<identifier>`.
		if idx := strings.LastIndex(d.Id(), componentIDSeparator); idx > 0 {
			projectID = d.Id()[:idx]
		}
	}

	components, err := api.ListComponents(ctx, client, projectID, &api.ComponentsFilter{
		Id: &api.IdFilter{Eq: d.Id()},
	})
	if err != nil {
		return diag.FromErr(err)
	}
	if len(components) == 0 {
		d.SetId("")
		return nil
	}

	component := components[0]
	d.Set("name", component.Name)
	d.Set("description", component.Description)
	d.Set("project_id", projectID)
	if component.OciRepo != nil {
		d.Set("oci_repo_name", component.OciRepo.Name)
	}

	prefix := projectID + componentIDSeparator
	if strings.HasPrefix(component.ID, prefix) {
		d.Set("identifier", strings.TrimPrefix(component.ID, prefix))
	}

	return nil
}

func resourceComponentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.RemoveComponent(ctx, client, d.Id()); err != nil {
		return diag.FromErr(fmt.Errorf("failed to remove component %s: %w", d.Id(), err))
	}

	d.SetId("")
	return nil
}
