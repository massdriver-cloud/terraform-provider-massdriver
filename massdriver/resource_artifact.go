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

	if c.EventTopicARN != "" {
		err = c.PublishEventToSNS(event)
		if err != nil {
			return diag.FromErr(err)
		}
	} else {
		artifactBytes, _ := json.Marshal(artifact)
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Development Override in effect. Artifact will not be created in Massdriver.",
			Detail:   string(artifactBytes),
		})
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

	if c.EventTopicARN != "" {
		err = c.PublishEventToSNS(event)
		if err != nil {
			return diag.FromErr(err)
		}
	} else {
		artifactBytes, _ := json.Marshal(artifact)
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Development Override in effect. Artifact will not be updated in Massdriver.",
			Detail:   string(artifactBytes),
		})
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

	if c.EventTopicARN != "" {
		err = c.PublishEventToSNS(event)
		if err != nil {
			return diag.FromErr(err)
		}
	} else {
		artifactBytes, _ := json.Marshal(artifact)
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Development Override in effect. Artifact will not be deleted from Massdriver.",
			Detail:   string(artifactBytes),
		})
	}

	d.SetId("")

	return diags
}

func validateArtifact(d *schema.ResourceData) error {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)

	schemaBytes, err := os.ReadFile(d.Get("schema_path").(string))
	if err != nil {
		return err
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
		return errors.New("artifact validation failed: unrecognized field: " + field)
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

func generateArtifact(d *schema.ResourceData) (map[string]interface{}, error) {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	name := d.Get("name").(string)
	providerResourceID := d.Get("provider_resource_id").(string)
	artifactType := d.Get("type").(string)

	// this here is a bit clunky. We're nesting the metadata object WITHIN the artifact. However, the schemas don't expect
	// the metadata block. So after validation (if it passes), we need to unmarshal the artifact to a map so we can
	// add the metadata in
	metadata := ArtifactMetadata{
		Field:              field,
		Name:               name,
		ProviderResourceID: providerResourceID,
		Type:               artifactType,
	}

	var unmarshaledArtifact map[string]interface{}
	err := json.Unmarshal([]byte(artifact), &unmarshaledArtifact)
	if err != nil {
		return unmarshaledArtifact, err
	}
	unmarshaledArtifact["metadata"] = metadata

	return unmarshaledArtifact, nil
}
