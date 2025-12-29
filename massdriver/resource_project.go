package massdriver

import (
	"context"
	"fmt"
	"time"

	"terraform-provider-massdriver/massdriver/services/projects"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceProject() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver project for organizing infrastructure and applications",

		CreateContext: resourceProjectCreate,
		ReadContext:   resourceProjectRead,
		UpdateContext: resourceProjectUpdate,
		DeleteContext: resourceProjectDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "The name of the project",
				Type:        schema.TypeString,
				Required:    true,
			},
			"slug": {
				Description: "The slug of the project (unique identifier)",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"description": {
				Description: "A description of the project",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
			},
			"last_updated": {
				Description: "A timestamp of when the last time this resource was updated",
				Type:        schema.TypeString,
				Optional:    false,
				Required:    false,
				Computed:    true,
			},
		},
	}
}

func resourceProjectCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ProjectService()

	var diags diag.Diagnostics

	name := d.Get("name").(string)
	slug := d.Get("slug").(string)
	description := d.Get("description").(string)

	project := &projects.Project{
		Name:        name,
		Slug:        slug,
		Description: description,
	}

	resp, createErr := service.CreateProject(ctx, project)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(resp.ID)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceProjectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ProjectService()

	var diags diag.Diagnostics

	project, getErr := service.GetProject(ctx, d.Id())
	if getErr != nil {
		if getErr.Error() == "not found" {
			d.SetId("")
			return diags
		}
		return diag.FromErr(getErr)
	}

	d.Set("name", project.Name)
	d.Set("slug", project.Slug)
	d.Set("description", project.Description)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceProjectUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ProjectService()

	var diags diag.Diagnostics

	if d.HasChanges("name", "description") {
		name := d.Get("name").(string)
		description := d.Get("description").(string)

		project := &projects.Project{
			ID:          d.Id(),
			Name:        name,
			Description: description,
		}

		_, updateErr := service.UpdateProject(ctx, project)
		if updateErr != nil {
			return diag.FromErr(updateErr)
		}

		d.Set("last_updated", time.Now().Format(time.RFC850))
	}

	return diags
}

func resourceProjectDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ProjectService()

	var diags diag.Diagnostics

	deleteErr := service.DeleteProject(ctx, d.Id())
	if deleteErr != nil {
		return diag.FromErr(fmt.Errorf("failed to delete project: %w", deleteErr))
	}

	d.SetId("")

	return diags
}
