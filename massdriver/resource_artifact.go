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
			"type": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceArtifactCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact, err := generateArtifact(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	event := NewEvent(EVENT_TYPE_ARTIFACT_CREATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err = c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(time.Now().Format(time.RFC3339))
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact, err := generateArtifact(ctx, d)
	if err != nil {
		return diag.FromErr(err)
	}

	event := NewEvent(EVENT_TYPE_ARTIFACT_UPDATED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err = c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	artifact := d.Get("artifact").(string)

	event := NewEvent(EVENT_TYPE_ARTIFACT_DELETED)
	event.Payload = EventPayloadArtifacts{DeploymentId: c.DeploymentID, Artifact: artifact}

	err := c.PublishEventToSNS(event)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}

func generateArtifact(ctx context.Context, d *schema.ResourceData) (string, error) {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	name := d.Get("name").(string)
	providerResourceID := d.Get("provider_resource_id").(string)
	artifactType := d.Get("type").(string)

	schemaBytes, err := os.ReadFile(d.Get("schema_path").(string))
	if err != nil {
		return "", err
	}

	// the schema-artifacts file has schemas for all of the artifacts in it (there can be more than one artifact).
	// We unmarshal all the schemas and pull out just the schema for this artifact to perform validation
	var schemaObj ArtifactSchema
	err = json.Unmarshal(schemaBytes, &schemaObj)
	if err != nil {
		return "", err
	}
	specificSchema, exists := schemaObj.Properties[field]
	if !exists {
		return "", errors.New("artifact validation failed: unrecognized field: " + field)
	}

	// Validate the artifact matches the schema
	valid, err := validate(specificSchema.(map[string]interface{}), artifact)
	if !valid || err != nil {
		return "", err
	}

	// this here is a bit clunky. We're nesting the metadata object WITHIN the artifact. However, the schemas don't expect
	// the metadata block. So after validation (if it passes), we need to unmarshal the artifact to a map so we can
	// add the metadata in and then remarshal.
	metadata := ArtifactMetadata{
		Field:              field,
		Name:               name,
		ProviderResourceID: providerResourceID,
		Type:               artifactType,
	}
	var unmarshaledArtifact map[string]interface{}
	err = json.Unmarshal([]byte(artifact), &unmarshaledArtifact)
	if err != nil {
		return "", err
	}
	unmarshaledArtifact["metadata"] = metadata
	remarshaledArtifact, err := json.Marshal(unmarshaledArtifact)
	if err != nil {
		return "", err
	}

	return string(remarshaledArtifact), nil
}

func validate(schema map[string]interface{}, artifact string) (bool, error) {

	sl := gojsonschema.NewGoLoader(schema)
	dl := gojsonschema.NewStringLoader(artifact)

	result, err := gojsonschema.Validate(sl, dl)
	if !result.Valid() {
		return false, errors.New("artifact validation failed: " + result.Errors()[0].String())
	}
	if err != nil {
		return false, err
	}

	return true, nil
}
