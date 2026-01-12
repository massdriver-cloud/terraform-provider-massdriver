package massdriver

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceEnvironment() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver environment within a project",

		CreateContext: resourceEnvironmentCreate,
		ReadContext:   resourceEnvironmentRead,
		UpdateContext: resourceEnvironmentUpdate,
		DeleteContext: resourceEnvironmentDelete,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Description: "The ID of the project this environment belongs to",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"name": {
				Description: "The name of the environment",
				Type:        schema.TypeString,
				Required:    true,
			},
			"slug": {
				Description: "The slug of the environment (unique within project, forms: project-slug)",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"description": {
				Description: "A description of the environment",
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

func resourceEnvironmentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	projectID := d.Get("project_id").(string)
	name := d.Get("name").(string)
	slug := d.Get("slug").(string)
	description := d.Get("description").(string)

	resp, createErr := createEnvironment(ctx, client.GQL, client.Config.OrganizationID, projectID, name, slug, description)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	if !resp.CreateEnvironment.Successful {
		messages := resp.CreateEnvironment.GetMessages()
		if len(messages) > 0 {
			errMsg := "unable to create environment:"
			for _, msg := range messages {
				errMsg += "\n  - " + msg.Message
			}
			return diag.FromErr(fmt.Errorf("%s", errMsg))
		}
		return diag.FromErr(fmt.Errorf("unable to create environment"))
	}

	d.SetId(resp.CreateEnvironment.Result.Id)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceEnvironmentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	resp, getErr := getEnvironmentById(ctx, client.GQL, client.Config.OrganizationID, d.Id())
	if getErr != nil {
		return diag.FromErr(getErr)
	}

	if resp.Environment.Id == "" {
		d.SetId("")
		return diags
	}

	d.Set("name", resp.Environment.Name)
	d.Set("slug", resp.Environment.Slug)
	d.Set("description", resp.Environment.Description)
	d.Set("project_id", resp.Environment.Project.Id)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceEnvironmentUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	if d.HasChanges("name", "description") {
		name := d.Get("name").(string)
		description := d.Get("description").(string)

		resp, updateErr := updateEnvironment(ctx, client.GQL, client.Config.OrganizationID, d.Id(), name, description)
		if updateErr != nil {
			return diag.FromErr(updateErr)
		}

		if !resp.UpdateEnvironment.Successful {
			messages := resp.UpdateEnvironment.GetMessages()
			if len(messages) > 0 {
				errMsg := "unable to update environment:"
				for _, msg := range messages {
					errMsg += "\n  - " + msg.Message
				}
				return diag.FromErr(fmt.Errorf("%s", errMsg))
			}
			return diag.FromErr(fmt.Errorf("unable to update environment"))
		}

		d.Set("last_updated", time.Now().Format(time.RFC850))
	}

	return diags
}

func resourceEnvironmentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	var diags diag.Diagnostics

	resp, deleteErr := deleteEnvironment(ctx, client.GQL, client.Config.OrganizationID, d.Id())
	if deleteErr != nil {
		return diag.FromErr(deleteErr)
	}

	if !resp.DeleteEnvironment.Successful {
		messages := resp.DeleteEnvironment.GetMessages()
		if len(messages) > 0 {
			errMsg := "unable to delete environment:"
			for _, msg := range messages {
				errMsg += "\n  - " + msg.Message
			}
			return diag.FromErr(fmt.Errorf("%s", errMsg))
		}
		return diag.FromErr(fmt.Errorf("unable to delete environment"))
	}

	d.SetId("")

	return diags
}
