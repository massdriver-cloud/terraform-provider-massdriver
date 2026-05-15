package massdriver

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceArtifact() *schema.Resource {
	return &schema.Resource{
		Description: `Looks up an existing Massdriver artifact by its identifier.

This data source enables inter-step data passing by allowing one bundle to read
artifacts produced by another bundle. This is useful for:

- Accessing infrastructure outputs from shared resources (VPCs, clusters, etc.)
- Reading credentials or connection details from other packages
- Building dependencies between packages that aren't directly linked on the canvas

## Artifact Identifier Format

The identifier format uses a dot to separate the package ID from the artifact field:

    {package_id}.{artifact_field}

Example: "api-prod-database.credentials"
`,

		ReadContext: dataSourceArtifactRead,

		Schema: map[string]*schema.Schema{
			"id": {
				Description: "The artifact identifier in the format `{package_id}.{field}` (e.g., `api-prod-db.credentials`).",
				Type:        schema.TypeString,
				Required:    true,
			},
			"name": {
				Description: "The human-readable name of the artifact.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"type": {
				Description: "The artifact type (artifact definition reference).",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"field": {
				Description: "The field name of the artifact within the source package.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"payload": {
				Description: "The artifact payload as a JSON string. Use `jsondecode()` to access individual fields.",
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

func dataSourceArtifactRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ArtifactService()

	var diags diag.Diagnostics

	id := d.Get("id").(string)

	artifact, err := service.GetArtifact(ctx, id)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(artifact.ID)
	d.Set("name", artifact.Name)
	d.Set("type", artifact.Type)
	d.Set("field", artifact.Field)

	// Serialize payload to JSON string
	if artifact.Payload != nil {
		payloadJSON, err := json.Marshal(artifact.Payload)
		if err != nil {
			return diag.FromErr(err)
		}
		d.Set("payload", string(payloadJSON))
	}

	return diags
}
