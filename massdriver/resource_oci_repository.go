package massdriver

import (
	"context"
	"errors"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/ocirepos"
)

// ociReposAPI is the slice of *ocirepos.Service this resource calls.
type ociReposAPI interface {
	Get(ctx context.Context, id string) (*ocirepos.OciRepo, error)
	Create(ctx context.Context, input ocirepos.CreateInput) (*ocirepos.OciRepo, error)
	Update(ctx context.Context, id string, input ocirepos.UpdateInput) (*ocirepos.OciRepo, error)
	Delete(ctx context.Context, id string) (*ocirepos.OciRepo, error)
}

var _ ociReposAPI = (*ocirepos.Service)(nil)

// ociRepositoryNamePattern enforces the server's repo-name rules: lowercase
// letters/digits/dashes/underscores, max 53 chars, at least 1 char. Catching
// these at plan time gives a faster, clearer failure than the server's reply.
var ociRepositoryNamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,53}$`)

func resourceOciRepository() *schema.Resource {
	return &schema.Resource{
		Description: "An OCI repository in the Massdriver catalog. Repositories must exist before any version can be published; pushing to a non-existent repository returns 404. The `artifact_type` field selects what kind of artifact the repository holds — today the only supported value is `BUNDLE`, but additional types (e.g. resource-type definitions, provisioner images) are planned.",

		CreateContext: resourceOciRepositoryCreate,
		ReadContext:   resourceOciRepositoryRead,
		UpdateContext: resourceOciRepositoryUpdate,
		DeleteContext: resourceOciRepositoryDelete,

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
					ociRepositoryNamePattern,
					"must be 1-53 characters, lowercase letters, digits, dashes, and underscores only",
				),
			},
			"artifact_type": {
				Description: "OCI artifact type this repository holds (e.g., `BUNDLE`). Immutable — repositories cannot be retyped. The server is the source of truth for valid values; new types added platform-side become usable here immediately. Plan-time validation is intentionally not performed against a client-side allowlist so the provider doesn't lag behind the platform.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"attributes": attributesSchema("OCI repository"),
			"reference": {
				Description: "Bare OCI reference for this repository (e.g., `api.massdriver.cloud/<org>/<name>`). Append `:<tag>` or `@<digest>` to address a specific manifest with `oras` or any OCI-compliant client.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceOciRepositoryCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	repo, err := pc.OciRepos.Create(ctx, ocirepos.CreateInput{
		ID:           d.Get("name").(string),
		ArtifactType: ocirepos.ArtifactType(d.Get("artifact_type").(string)),
		Attributes:   attributesFromConfig(d.Get("attributes")),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(repo.ID)
	return resourceOciRepositoryRead(ctx, d, meta)
}

func resourceOciRepositoryRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	repo, err := pc.OciRepos.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("name", repo.Name)
	d.Set("reference", repo.Reference)
	// ArtifactType is a typed enum on the SDK side (normalized from the
	// server's media-type string); the schema field is TypeString, so cast
	// at the boundary.
	d.Set("artifact_type", string(repo.ArtifactType))
	d.Set("attributes", attributesToState(repo.Attributes))
	return nil
}

func resourceOciRepositoryUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.OciRepos.Update(ctx, d.Id(), ocirepos.UpdateInput{
		Attributes: attributesFromConfig(d.Get("attributes")),
	}); err != nil {
		return diag.FromErr(err)
	}

	return resourceOciRepositoryRead(ctx, d, meta)
}

func resourceOciRepositoryDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.OciRepos.Delete(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
