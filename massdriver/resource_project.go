package massdriver

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/projects"
)

// projectsAPI is the slice of *projects.Service that this resource calls.
// *projects.Service satisfies it in production; tests inject fakes.
type projectsAPI interface {
	Get(ctx context.Context, id string) (*projects.Project, error)
	Create(ctx context.Context, input projects.CreateInput) (*projects.Project, error)
	Update(ctx context.Context, id string, input projects.UpdateInput) (*projects.Project, error)
	Delete(ctx context.Context, id string) (*projects.Project, error)
}

// Compile-time assertion that the SDK satisfies our interface.
var _ projectsAPI = (*projects.Service)(nil)

func resourceProject() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver project — a logical grouping of environments and components.",

		CreateContext: resourceProjectCreate,
		ReadContext:   resourceProjectRead,
		UpdateContext: resourceProjectUpdate,
		DeleteContext: resourceProjectDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"identifier": identifierSchema("project"),
			"name": {
				Description: "Human-readable name for the project. Defaults to `identifier` if unset. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"description": {
				Description: "Optional description of the project's purpose. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"attributes": attributesSchema("project"),
		},
	}
}

func resourceProjectCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	identifier := d.Get("identifier").(string)
	name := d.Get("name").(string)
	if name == "" {
		name = identifier
	}

	project, err := pc.Projects.Create(ctx, projects.CreateInput{
		ID:          identifier,
		Name:        name,
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(project.ID)
	return resourceProjectRead(ctx, d, meta)
}

func resourceProjectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	project, err := pc.Projects.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	// For projects the platform ID and the user-supplied identifier are the same.
	d.Set("identifier", project.ID)
	d.Set("name", project.Name)
	d.Set("description", project.Description)
	d.Set("attributes", attributesToState(project.Attributes))
	return nil
}

func resourceProjectUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Projects.Update(ctx, d.Id(), projects.UpdateInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceProjectRead(ctx, d, meta)
}

func resourceProjectDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.Projects.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
