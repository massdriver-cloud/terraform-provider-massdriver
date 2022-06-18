package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

type ArtifactMetadata struct {
	Field              string `json:"field"`
	ProviderResourceID string `json:"provider_resource_id"`
	Type               string `json:"type"`
	Name               string `json:"name"`
}

type ArtifactSchema struct {
	Properties map[string]interface{} `json:"properties"`
}

type BundleSpecification struct {
	Artifacts map[string]ArtifactSpecification `yaml:"artifacts"`
}
type ArtifactSpecification map[string]string

func resourceArtifact() *schema.Resource {
	return &schema.Resource{

		CreateContext: resourceArtifactCreate,
		ReadContext:   schema.NoopContext,
		UpdateContext: resourceArtifactUpdate,
		DeleteContext: resourceArtifactDelete,

		Schema: map[string]*schema.Schema{
			"artifact": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},
			"field": {
				Type:     schema.TypeString,
				Required: true,
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"provider_resource_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"schema_path": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "../schema-artifacts.json",
			},
			// need this for now to lookup what "type" the artifact is from the spec
			"specification_path": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "../massdriver.yaml",
			},
			"type": {
				Type:       schema.TypeString,
				Optional:   true,
				Default:    "",
				Deprecated: "This field is being removed and instead the type is fetched from the massdriver.yaml file",
			},
		},
	}
}

func resourceArtifactCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	err := validateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	artifact, err := generateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	event := NewEvent(EVENT_TYPE_ARTIFACT_CREATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err = c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId(time.Now().Format(time.RFC3339))
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return diags
}

func resourceArtifactUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	err := validateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	artifact, err := generateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	event := NewEvent(EVENT_TYPE_ARTIFACT_UPDATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err = c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact, err := generateArtifact(d)
	if err != nil {
		return diag.FromErr(err)
	}

	event := NewEvent(EVENT_TYPE_ARTIFACT_DELETED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err = c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId("")

	return diags
}

func validateArtifact(d *schema.ResourceData) error {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	schemaPath := d.Get("schema_path").(string)

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

	specificationBytes, err := os.ReadFile(specificationPath)
	if err != nil {
		return "", errors.New(`Unable to open specification file: ` + specificationPath)
	}

	var bundleSpec BundleSpecification
	err = yaml.Unmarshal(specificationBytes, &bundleSpec)
	if err != nil {
		return "", err
	}
	artifactSpec, exists := bundleSpec.Artifacts[field]
	if !exists {
		return "", errors.New(`artifact validation failed: field "` + field + `" does not exist in specification`)
	}

	artifactType, exists := artifactSpec["$ref"]
	if !exists {
		return "", errors.New(`artifact validation failed: field "` + field + `" does not contain a $ref`)
	}

	return artifactType, nil
}

func generateArtifact(d *schema.ResourceData) (map[string]interface{}, error) {
	var unmarshaledArtifact map[string]interface{}

	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	name := d.Get("name").(string)
	providerResourceID := d.Get("provider_resource_id").(string)
	artifactType, err := getArtifactType(d)
	if err != nil {
		return unmarshaledArtifact, err
	}

	// this here is a bit clunky. We're nesting the metadata object WITHIN the artifact. However, the schemas don't expect
	// the metadata block. So after validation (if it passes), we need to unmarshal the artifact to a map so we can
	// add the metadata in
	metadata := ArtifactMetadata{
		Field:              field,
		Name:               name,
		ProviderResourceID: providerResourceID,
		Type:               artifactType,
	}

	err = json.Unmarshal([]byte(artifact), &unmarshaledArtifact)
	if err != nil {
		return unmarshaledArtifact, err
	}
	unmarshaledArtifact["metadata"] = metadata

	return unmarshaledArtifact, nil
}
