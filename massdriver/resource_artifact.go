package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/artifacts"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"
)

const DEFAULT_ARTIFACT_SCHEMA_PATH = "../schema-artifacts.json"
const DEFAULT_SPECIFICATION_PATH = "../massdriver.yaml"

type ArtifactSchema struct {
	Properties map[string]interface{} `json:"properties"`
}

// BundleSpecification is the relevant slice of massdriver.yaml. The artifact
// type comes from `resources.<field>.resource_type`, falling back to the
// legacy `artifacts.properties.<field>.$ref`.
type BundleSpecification struct {
	Resources map[string]ResourceSpecification `yaml:"resources"`
	Artifacts ArtifactSpecification            `yaml:"artifacts"`
}

// ResourceSpecification is an entry in the modern `resources` map:
//
//	resources:
//	  my_field:
//	    resource_type: my-org/my-type
//	    required: true
type ResourceSpecification struct {
	ResourceType string `json:"resource_type" yaml:"resource_type"`
	Required     bool   `json:"required" yaml:"required"`
}

// ArtifactSpecification is the legacy `artifacts` block:
//
//	artifacts:
//	  properties:
//	    my_field:
//	      $ref: my-type
type ArtifactSpecification struct {
	Properties map[string]map[string]string `json:"properties" yaml:"properties"`
}

func resourceArtifact() *schema.Resource {
	return &schema.Resource{
		Description:        "A Massdriver artifact for exporting a connectable type",
		DeprecationMessage: "massdriver_artifact is deprecated and will be removed in v2.0 of the massdriver provider. Use `massdriver_resource` instead. Do not manage the same record via both `massdriver_artifact` and `massdriver_resource` — terraform will not detect the conflict and the two resources will fight over state.",

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
				Description: "The name of this artifact. Must match the field name given to it in the massdriver.yaml file — either a key under `resources` or, in the legacy layout, under `artifacts.properties`.",
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
				Description: "The path to the schema-artifacts.json file used for client-side JSON Schema validation of the artifact. The Massdriver CLI no longer emits this file; when it is absent, client-side validation is skipped and the artifact is validated server-side instead. Retained for backwards compatibility — this value should only ever be changed when doing local provider testing.",
				Type:        schema.TypeString,
				Optional:    true,
				Default:     DEFAULT_ARTIFACT_SCHEMA_PATH,
			},
			// need this for now to lookup what "type" the artifact is from the spec
			"specification_path": {
				Description: "The path to the massdriver.yaml file in order to lookup the type used for this artifact. Reads `resources.<field>.resource_type`, falling back to the legacy `artifacts.properties.<field>.$ref`. This value should only ever be changed when doing local provider testing.",
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

	artifact, err := generateArtifact(d, meta.(*ProviderClient).Client)
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

	artifact, err := generateArtifact(d, meta.(*ProviderClient).Client)
	if err != nil {
		return diag.FromErr(err)
	}

	_, updateErr := service.UpdateArtifact(ctx, getID(d), artifact)
	if updateErr != nil {
		return diag.FromErr(updateErr)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceArtifactDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	service := meta.(*ProviderClient).ArtifactService()

	var diags diag.Diagnostics

	id := getID(d)
	field := d.Get("field").(string)

	deleteErr := service.DeleteArtifact(ctx, id, field)
	if deleteErr != nil {
		return diag.FromErr(deleteErr)
	}

	d.SetId("")

	return diags
}

func getID(d *schema.ResourceData) string {
	artifactID := d.Id()

	// If the ID is a timestamp, it was from the older system where we didn't have IDs. We need to convert the ID to the new format, which is <package_name>-<field>
	if _, err := time.Parse(time.RFC3339, artifactID); err == nil {
		packageName := os.Getenv("MASSDRIVER_PACKAGE_NAME")
		artifactID = fmt.Sprintf("%s-%s", packageName, d.Get("field"))
	}
	return artifactID
}

// validateArtifact validates the artifact against the JSON Schema for this
// field in schema-artifacts.json, when that schema is available.
//
// The Massdriver CLI no longer emits schema-artifacts.json, and artifacts are
// validated server-side, so an unavailable schema is not an error: we skip
// client-side validation and let the API be the authority. Only a schema we
// can actually load and resolve for this field is enforced.
func validateArtifact(d *schema.ResourceData) error {
	artifact := d.Get("artifact").(string)
	field := d.Get("field").(string)
	schemaPath := d.Get("schema_path").(string)
	if schemaPath == "" {
		schemaPath = DEFAULT_ARTIFACT_SCHEMA_PATH
	}

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		// No schema file (the common case since the CLI stopped emitting it).
		return nil
	}

	// the schema-artifacts file has schemas for all of the artifacts in it (there can be more than one artifact).
	// We unmarshal all the schemas and pull out just the schema for this artifact to perform validation
	var schemaObj ArtifactSchema
	if err := json.Unmarshal(schemaBytes, &schemaObj); err != nil {
		// Unreadable schema file; fall back to server-side validation.
		return nil
	}
	specificSchema, exists := schemaObj.Properties[field]
	if !exists {
		// No schema for this field; fall back to server-side validation.
		return nil
	}
	schemaMap, ok := specificSchema.(map[string]interface{})
	if !ok {
		// Not an object schema (e.g. a boolean); nothing we can enforce here.
		return nil
	}

	// Validate
	sl := gojsonschema.NewGoLoader(schemaMap)
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
func getArtifactType(d *schema.ResourceData, mdClient *client.Client) (string, error) {
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

	artifactType, err := lookupArtifactType(bundleSpec, field)
	if err != nil {
		return "", err
	}

	split := strings.Split(artifactType, "/")
	if len(split) != 2 {
		artifactType = strings.Join([]string{mdClient.Config.OrganizationID, artifactType}, "/")
	}

	return artifactType, nil
}

// lookupArtifactType resolves a field's type from massdriver.yaml, preferring
// the modern `resources` block and falling back to the legacy `artifacts`
// block so bundles written against either layout keep working.
func lookupArtifactType(bundleSpec BundleSpecification, field string) (string, error) {
	if resourceSpec, exists := bundleSpec.Resources[field]; exists {
		if resourceSpec.ResourceType == "" {
			return "", errors.New(`artifact validation failed: field "` + field + `" has an empty resource_type`)
		}
		return resourceSpec.ResourceType, nil
	}

	if artifactSpec, exists := bundleSpec.Artifacts.Properties[field]; exists {
		artifactType, hasRef := artifactSpec["$ref"]
		if !hasRef || artifactType == "" {
			return "", errors.New(`artifact validation failed: field "` + field + `" does not contain a $ref`)
		}
		return artifactType, nil
	}

	return "", errors.New(`artifact validation failed: field "` + field + `" does not exist in specification`)
}

func generateArtifact(d *schema.ResourceData, mdClient *client.Client) (*artifacts.Artifact, error) {
	artifact := artifacts.Artifact{}

	artifactString := d.Get("artifact").(string)
	artifact.Field = d.Get("field").(string)
	artifact.Name = d.Get("name").(string)

	var typeErr error
	artifact.Type, typeErr = getArtifactType(d, mdClient)
	if typeErr != nil {
		return nil, typeErr
	}

	// Unmarshal the user's artifact JSON into a map for the payload
	var payload map[string]interface{}
	unmarshalErr := json.Unmarshal([]byte(artifactString), &payload)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	// Set the payload field - this is the new format that the API expects
	artifact.Payload = payload

	// Extract "data" and "specs" from the payload if they exist
	if data, dataExists := payload["data"]; dataExists {
		artifact.Data = data.(map[string]interface{})
	}
	if specs, specsExists := payload["specs"]; specsExists {
		artifact.Specs = specs.(map[string]interface{})
	}

	return &artifact, nil
}
