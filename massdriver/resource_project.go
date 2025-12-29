package massdriver

import (
	"context"
	"fmt"
	"time"

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
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	name := d.Get("name").(string)
	slug := d.Get("slug").(string)
	description := d.Get("description").(string)

	resp, createErr := createProject(ctx, client.GQL, client.Config.OrganizationID, name, slug, description)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	if !resp.CreateProject.Successful {
		messages := resp.CreateProject.GetMessages()
		if len(messages) > 0 {
			errMsg := "unable to create project:"
			for _, msg := range messages {
				errMsg += "\n  - " + msg.Message
			}
			return diag.FromErr(fmt.Errorf("%s", errMsg))
		}
		return diag.FromErr(fmt.Errorf("unable to create project"))
	}

	d.SetId(resp.CreateProject.Result.Id)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceProjectRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	resp, getErr := getProjectById(ctx, client.GQL, client.Config.OrganizationID, d.Id())
	if getErr != nil {
		return diag.FromErr(getErr)
	}

	if resp.Project.Id == "" {
		d.SetId("")
		return diags
	}

	d.Set("name", resp.Project.Name)
	d.Set("slug", resp.Project.Slug)
	d.Set("description", resp.Project.Description)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceProjectUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	if d.HasChanges("name", "description") {
		name := d.Get("name").(string)
		description := d.Get("description").(string)

		resp, updateErr := updateProject(ctx, client.GQL, client.Config.OrganizationID, d.Id(), name, description)
		if updateErr != nil {
			return diag.FromErr(updateErr)
		}

		if !resp.UpdateProject.Successful {
			messages := resp.UpdateProject.GetMessages()
			if len(messages) > 0 {
				errMsg := "unable to update project:"
				for _, msg := range messages {
					errMsg += "\n  - " + msg.Message
				}
				return diag.FromErr(fmt.Errorf("%s", errMsg))
			}
			return diag.FromErr(fmt.Errorf("unable to update project"))
		}

		d.Set("last_updated", time.Now().Format(time.RFC850))
	}

	return diags
}

func resourceProjectDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	resp, deleteErr := deleteProject(ctx, client.GQL, client.Config.OrganizationID, d.Id())
	if deleteErr != nil {
		return diag.FromErr(deleteErr)
	}

	if !resp.DeleteProject.Successful {
		messages := resp.DeleteProject.GetMessages()
		if len(messages) > 0 {
			errMsg := "unable to delete project:"
			for _, msg := range messages {
				errMsg += "\n  - " + msg.Message
			}
			return diag.FromErr(fmt.Errorf("%s", errMsg))
		}
		return diag.FromErr(fmt.Errorf("unable to delete project"))
	}

	d.SetId("")

	return diags
}
