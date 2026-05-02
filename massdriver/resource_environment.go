package massdriver

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

func resourceEnvironment() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver environment within a project (e.g., prod, staging).",

		CreateContext: resourceEnvironmentCreate,
		ReadContext:   resourceEnvironmentRead,
		UpdateContext: resourceEnvironmentUpdate,
		DeleteContext: resourceEnvironmentDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"identifier": identifierSchema("environment"),
			"project_id": {
				Description: "ID of the project this environment belongs to.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"name": {
				Description: "Human-readable name for the environment. Defaults to `identifier` if unset. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"description": {
				Description: "Optional description of the environment's purpose. When unset, drift on this field (e.g., a console edit) is ignored.",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"attributes": attributesSchema("environment"),
		},
	}
}

func resourceEnvironmentCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	identifier := d.Get("identifier").(string)
	name := d.Get("name").(string)
	if name == "" {
		name = identifier
	}

	env, err := api.CreateEnvironment(ctx, client, d.Get("project_id").(string), api.CreateEnvironmentInput{
		Id:          identifier,
		Name:        name,
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(env.ID)
	return resourceEnvironmentRead(ctx, d, meta)
}

func resourceEnvironmentRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	env, err := api.GetEnvironment(ctx, client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", env.Name)
	d.Set("description", env.Description)
	d.Set("attributes", attributesToState(env.Attributes))

	projectID := ""
	if env.Project != nil {
		projectID = env.Project.ID
		d.Set("project_id", projectID)
	}
	// Platform IDs follow `<project>-<env>`; recover the user-supplied identifier
	// by stripping the project prefix.
	if projectID != "" && strings.HasPrefix(env.ID, projectID+"-") {
		d.Set("identifier", strings.TrimPrefix(env.ID, projectID+"-"))
	}
	return nil
}

func resourceEnvironmentUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	_, err := api.UpdateEnvironment(ctx, client, d.Id(), api.UpdateEnvironmentInput{
		Name:        d.Get("name").(string),
		Description: d.Get("description").(string),
		Attributes:  attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceEnvironmentRead(ctx, d, meta)
}

func resourceEnvironmentDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteEnvironment(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
