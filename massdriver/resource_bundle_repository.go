package massdriver

import (
	"context"
	"regexp"

	"terraform-provider-massdriver/internal/api"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// bundleRepositoryNamePattern enforces the server's repo-name rules: lowercase
// letters/digits/dashes/underscores, max 53 chars, at least 1 char. Catching
// these at plan time gives a faster, clearer failure than the server's reply.
var bundleRepositoryNamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,53}$`)

func resourceBundleRepository() *schema.Resource {
	return &schema.Resource{
		Description: "A bundle repository in the Massdriver catalog. Repositories must exist before any version can be published; pushing to a non-existent repository returns 404.",

		CreateContext: resourceBundleRepositoryCreate,
		ReadContext:   resourceBundleRepositoryRead,
		UpdateContext: resourceBundleRepositoryUpdate,
		DeleteContext: resourceBundleRepositoryDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Unique repository name within your organization (e.g., `aws-aurora-postgres`). Lowercase letters, numbers, dashes, and underscores only. Max 53 characters. Immutable.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				ValidateFunc: validation.StringMatch(
					bundleRepositoryNamePattern,
					"must be 1-53 characters, lowercase letters, digits, dashes, and underscores only",
				),
			},
			"attributes": attributesSchema("bundle repository"),
			"reference": {
				Description: "Bare OCI reference for this repository (e.g., `api.massdriver.cloud/<org>/<name>`). Append `:<tag>` or `@<digest>` to address a specific manifest with `oras` or any OCI-compliant client.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"artifact_type": {
				Description: "OCI artifact type stored in this repository (currently always `application/vnd.massdriver.bundle.v1+json`).",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceBundleRepositoryCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	repo, err := api.CreateOciRepo(ctx, client, api.CreateOciRepoInput{
		Id:           d.Get("name").(string),
		ArtifactType: api.OciArtifactTypeBundle,
		Attributes:   attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(repo.ID)
	return resourceBundleRepositoryRead(ctx, d, meta)
}

func resourceBundleRepositoryRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	repo, err := api.GetOciRepo(ctx, client, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", repo.Name)
	d.Set("reference", repo.Reference)
	d.Set("artifact_type", repo.ArtifactType)
	d.Set("attributes", attributesToState(repo.Attributes))
	return nil
}

func resourceBundleRepositoryUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	_, err := api.UpdateOciRepo(ctx, client, d.Id(), api.UpdateOciRepoInput{
		Attributes: attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceBundleRepositoryRead(ctx, d, meta)
}

func resourceBundleRepositoryDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteOciRepo(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
