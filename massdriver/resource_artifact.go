package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/artifacts"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

const DEFAULT_ARTIFACT_SCHEMA_PATH = "../schema-artifacts.json"
const DEFAULT_SPECIFICATION_PATH = "../massdriver.yaml"

type ArtifactSchema struct {
	Properties map[string]interface{} `json:"properties"`
}

type BundleSpecification struct {
	Artifacts ArtifactSpecification `yaml:"artifacts"`
}

type ArtifactSpecification struct {
	Properties map[string]map[string]string `json:"properties" yaml:"properties"`
}

func resourceArtifact() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver artifact for exporting a connectable type",

		CreateContext: resourceArtifactCreate,
		ReadContext:   schema.NoopContext,
		UpdateContext: resourceArtifactUpdate,
		DeleteContext: resourceArtifactDelete,

		Schema: map[string]*schema.Schema{
			"artifact": {
				Description: "A json formatted string containing the artifact.",
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
			},
			"field": {
				Description: "The name of this artifact. Must match the name given to this artifact in the massdriver.yaml file.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"last_updated": {
				Description: "A timestamp of when the last time this resource was updated",
				Type:        schema.TypeString,
				Optional:    false,
				Required:    false,
				Computed:    true,
			},
			"name": {
				Description: "A human readable name for this artifact.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"provider_resource_id": {
				Description: "An cloud identifier (AWS ARN, Google/Azure ID) for the primary resource this bundle creates.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Deprecated:  "This field is deprecated and will be removed in a future version.",
			},
			"schema_path": {
				Description: "The path to the schema-artifacts.json file in order to perform JSON Schema validation on the artifact before sending to Massdriver. This value should only ever be changed when doing local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     DEFAULT_ARTIFACT_SCHEMA_PATH,
			},
			// need this for now to lookup what "type" the artifact is from the spec
			"specification_path": {
				Description: "The path to the massdriver.yaml file in order to lookup the schema type used for this artifact. This value should only ever be changed when doing local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     DEFAULT_SPECIFICATION_PATH,
			},
			"type": {
				Description: "This value is deprecated and should no longer be used. It is ignored in the provider code.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Deprecated:  "This field is being removed and instead the type is fetched from the massdriver.yaml file",
			},
		},
	}
}

func resourceArtifactCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ArtifactService()

	var diags diag.Diagnostics

	err := validateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	artifact, err := generateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	resp, createErr := service.CreateArtifact(ctx, artifact)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(resp.ID)
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return diags
}

func resourceArtifactUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ArtifactService()

	var diags diag.Diagnostics

	err := validateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	artifact, err := generateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	_, updateErr := service.UpdateArtifact(ctx, d.Id(), artifact)
	if updateErr != nil {
		return diag.FromErr(updateErr)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ArtifactService()

	var diags diag.Diagnostics

	id := d.Id()
	field := d.Get("field").(string)

	deleteErr := service.DeleteArtifact(ctx, id, field)
	if deleteErr != nil {
		return diag.FromErr(deleteErr)
	}

	d.SetId("")

	return diags
}

func validateArtifact(d *schema.ResourceData) error {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	schemaPath := d.Get("schema_path").(string)
	if schemaPath == "" {
		schemaPath = DEFAULT_ARTIFACT_SCHEMA_PATH
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return errors.New(`Unable to open schema file: ` + schemaPath)
	}

	// the schema-artifacts file has schemas for all of the artifacts in it (there can be more than one artifact).
	// We unmarshal all the schemas and pull out just the schema for this artifact to perform validation
	var schemaObj ArtifactSchema
	err = json.Unmarshal(schemaBytes, &schemaObj)
	if err != nil {
		return err
	}
	specificSchema, exists := schemaObj.Properties[field]
	if !exists {
		return errors.New(`artifact validation failed: field "` + field + `" does not exist in schema`)
	}

	// Validate
	sl := gojsonschema.NewGoLoader(specificSchema.(map[string]interface{}))
	dl := gojsonschema.NewStringLoader(artifact)

	result, err := gojsonschema.Validate(sl, dl)
	if err != nil {
		return err
	}
	if !result.Valid() {
		return errors.New("artifact validation failed: " + result.Errors()[0].String())
	}

	return nil
}

// For now we need to fetch the type from the massdriver.yaml file
func getArtifactType(d *schema.ResourceData) (string, error) {
	field := d.Get("field").(string)
	specificationPath := d.Get("specification_path").(string)
	if specificationPath == "" {
		specificationPath = DEFAULT_SPECIFICATION_PATH
	}

	specificationBytes, err := os.ReadFile(specificationPath)
	if err != nil {
		return "", errors.New(`Unable to open specification file: ` + specificationPath)
	}

	var bundleSpec BundleSpecification
	err = yaml.Unmarshal(specificationBytes, &bundleSpec)
	if err != nil {
		return "", err
	}

	artifactSpec, exists := bundleSpec.Artifacts.Properties[field]
	if !exists {
		return "", errors.New(`artifact validation failed: field "` + field + `" does not exist in specification`)
	}

	artifactType, exists := artifactSpec["$ref"]
	if !exists {
		return "", errors.New(`artifact validation failed: field "` + field + `" does not contain a $ref`)
	}

	return artifactType, nil
}

func generateArtifact(d *schema.ResourceData) (*artifacts.Artifact, error) {
	artifact := artifacts.Artifact{}
	metadata := artifacts.Metadata{}

	artifactString := d.Get("artifact").(string)
	metadata.Field = d.Get("field").(string)
	metadata.Name = d.Get("name").(string)

	var typeErr error
	metadata.Type, typeErr = getArtifactType(d)
	if typeErr != nil {
		return nil, typeErr
	}

	unmarshalErr := json.Unmarshal([]byte(artifactString), &artifact)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	artifact.Metadata = &metadata

	return &artifact, nil
}
