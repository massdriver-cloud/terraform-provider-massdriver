package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceProjectCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createProject": {
			"data": map[string]any{
				"createProject": map[string]any{
					"result": map[string]any{
						"id":          "ecomm",
						"name":        "Ecomm Project",
						"description": "the e-commerce app",
						"attributes":  map[string]any{"team": "platform"},
					},
					"successful": true,
				},
			},
		},
		"getProject": {
			"data": map[string]any{
				"project": map[string]any{
					"id":          "ecomm",
					"name":        "Ecomm Project",
					"description": "the e-commerce app",
					"attributes":  map[string]any{"team": "platform"},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier":  "ecomm",
		"name":        "Ecomm Project",
		"description": "the e-commerce app",
		"attributes":  map[string]any{"team": "platform"},
	})

	diags := resourceProjectCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm" {
		t.Errorf("got id %q, want %q", rd.Id(), "ecomm")
	}
	if got := rd.Get("identifier").(string); got != "ecomm" {
		t.Errorf("got identifier %q, want ecomm", got)
	}
	if got := rd.Get("name").(string); got != "Ecomm Project" {
		t.Errorf("got name %q, want %q", got, "Ecomm Project")
	}
	if got := rd.Get("description").(string); got != "the e-commerce app" {
		t.Errorf("got description %q, want %q", got, "the e-commerce app")
	}

	createReq := rec.FindRequest("createProject")
	if createReq == nil {
		t.Fatal("createProject was not called")
	}
	vars := gqlmock.Variables(createReq)
	if vars["organizationId"] != testOrgID {
		t.Errorf("got organizationId %v, want %s", vars["organizationId"], testOrgID)
	}
	input, _ := vars["input"].(map[string]any)
	// The user-supplied "identifier" attribute is sent as the API's "id" field.
	if input["id"] != "ecomm" {
		t.Errorf("got input.id %v, want ecomm", input["id"])
	}
	if input["name"] != "Ecomm Project" {
		t.Errorf("got input.name %v, want Ecomm Project", input["name"])
	}
	if input["description"] != "the e-commerce app" {
		t.Errorf("got input.description %v, want the e-commerce app", input["description"])
	}
	// attributes is the JSON-scalar (double-encoded) — wire payload is a JSON-encoded string.
	if input["attributes"] != `{"team":"platform"}` {
		t.Errorf("got input.attributes %v, want JSON-encoded team=platform", input["attributes"])
	}

	if rec.FindRequest("getProject") == nil {
		t.Error("Read was not called after Create")
	}

	// Read populates attributes back into state.
	if attrs := rd.Get("attributes").(map[string]any); attrs["team"] != "platform" {
		t.Errorf("got attributes %v after Read, wanted team=platform", attrs)
	}
}

// Empty attributes must be omitted from the wire — the server rejects
// `attributes: null` (which is what the JSON-scalar marshaler used to send for
// nil maps before the omit-on-empty fix).
func TestResourceProjectCreateOmitsEmptyAttributes(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createProject": {
			"data": map[string]any{
				"createProject": map[string]any{
					"result":     map[string]any{"id": "ecomm", "name": "Ecomm"},
					"successful": true,
				},
			},
		},
		"getProject": {
			"data": map[string]any{
				"project": map[string]any{"id": "ecomm", "name": "Ecomm"},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		"name":       "Ecomm",
		"attributes": map[string]any{},
	})

	if diags := resourceProjectCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createProject"))["input"].(map[string]any)
	if _, present := input["attributes"]; present {
		t.Errorf("attributes should be omitted from wire when empty, got %v", input["attributes"])
	}
}

// Drift on attributes MUST surface — they drive permissions, so silent drift
// would be a security issue. Mirrors the equivalent component test.
func TestResourceProjectSurfacesAttributesDrift(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":              "ecomm",
			"identifier":      "ecomm",
			"attributes.%":    "1",
			"attributes.team": "infra", // drifted via console edit
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
		"attributes": map[string]any{"team": "platform"},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected attributes drift to surface as a diff")
	}
	if attr := diff.Attributes["attributes.team"]; attr == nil {
		t.Error("expected diff on attributes.team")
	} else if attr.Old != "infra" || attr.New != "platform" {
		t.Errorf("got attributes.team diff %+v, want Old=infra New=platform", attr)
	}
}

func TestResourceProjectCreatePropagatesAPIFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"createProject": {
			"data": map[string]any{
				"createProject": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "validation", "field": "id", "message": "id already exists"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		"name":       "Ecomm Project",
	})

	diags := resourceProjectCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from failed mutation, got none")
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceProjectRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getProject": {
			"data": map[string]any{
				"project": map[string]any{
					"id":          "ecomm",
					"name":        "Ecomm Project",
					"description": "from the server",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("ecomm")

	diags := resourceProjectRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("identifier").(string) != "ecomm" {
		// For projects identifier == platform id.
		t.Errorf("got identifier %q, want ecomm", rd.Get("identifier"))
	}
	if rd.Get("name").(string) != "Ecomm Project" {
		t.Errorf("got name %q, want Ecomm Project", rd.Get("name"))
	}
	if rd.Get("description").(string) != "from the server" {
		t.Errorf("got description %q, want %q", rd.Get("description"), "from the server")
	}
}

func TestResourceProjectUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateProject": {
			"data": map[string]any{
				"updateProject": map[string]any{
					"result": map[string]any{
						"id":          "ecomm",
						"name":        "Renamed",
						"description": "updated",
					},
					"successful": true,
				},
			},
		},
		"getProject": {
			"data": map[string]any{
				"project": map[string]any{
					"id":          "ecomm",
					"name":        "Renamed",
					"description": "updated",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier":  "ecomm",
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("ecomm")

	diags := resourceProjectUpdate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateProject")
	if updateReq == nil {
		t.Fatal("updateProject was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "ecomm" {
		t.Errorf("got id arg %v, want ecomm", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Renamed" || input["description"] != "updated" {
		t.Errorf("got input %v, want name=Renamed description=updated", input)
	}
}

func TestResourceProjectDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteProject": {
			"data": map[string]any{
				"deleteProject": map[string]any{
					"result":     map[string]any{"id": "ecomm", "name": "Ecomm"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{})
	rd.SetId("ecomm")

	diags := resourceProjectDelete(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared after delete, got %q", rd.Id())
	}

	deleteReq := rec.FindRequest("deleteProject")
	if deleteReq == nil {
		t.Fatal("deleteProject was not called")
	}
	if vars := gqlmock.Variables(deleteReq); vars["id"] != "ecomm" {
		t.Errorf("got id %v, want ecomm", vars["id"])
	}
}

func TestResourceProjectCreateDefaultsNameToIdentifier(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createProject": {
			"data": map[string]any{
				"createProject": map[string]any{
					"result": map[string]any{
						"id":   "ecomm",
						"name": "ecomm",
					},
					"successful": true,
				},
			},
		},
		"getProject": {
			"data": map[string]any{
				"project": map[string]any{
					"id":   "ecomm",
					"name": "ecomm",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceProject().Schema, map[string]any{
		"identifier": "ecomm",
		// name and description deliberately omitted
	})

	diags := resourceProjectCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	createReq := rec.FindRequest("createProject")
	if createReq == nil {
		t.Fatal("createProject was not called")
	}
	input, _ := gqlmock.Variables(createReq)["input"].(map[string]any)
	// API requires a name — when the user didn't supply one we substitute the identifier.
	if input["name"] != "ecomm" {
		t.Errorf("got input.name %v, want ecomm (defaulted from identifier)", input["name"])
	}
	if input["description"] != "" {
		t.Errorf("got input.description %v, want empty", input["description"])
	}
}

// When the user omits name/description from config, terraform's diff resolver
// must keep whatever is currently in state — that's how console edits avoid
// being reverted on the next apply. This is the load-bearing behavior the user
// asked for, so it's worth testing through the actual diff machinery rather
// than by inspection.
func TestResourceProjectIgnoresDriftWhenConfigUnset(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":          "ecomm",
			"identifier":  "ecomm",
			"name":        "Console Edit",        // someone changed it in the UI after Read
			"description": "added in the console", // ditto
		},
	}
	// Config omits name and description entirely — only required attrs are present.
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}

	if diff != nil && !diff.Empty() {
		for k, attr := range diff.Attributes {
			if k == "name" || k == "description" {
				t.Errorf("expected no diff on %s when config omits it; got %+v", k, attr)
			}
		}
	}
}

// Conversely, if the user DOES specify a value, drift must surface as a real
// diff so terraform can plan an update.
func TestResourceProjectShowsDriftWhenConfigSet(t *testing.T) {
	r := resourceProject()

	state := &terraform.InstanceState{
		ID: "ecomm",
		Attributes: map[string]string{
			"id":         "ecomm",
			"identifier": "ecomm",
			"name":       "Console Edit",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "ecomm",
		"name":       "Tracked By Terraform",
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	if diff == nil || diff.Empty() {
		t.Fatal("expected a diff because config sets a value that differs from state")
	}
	if attr := diff.Attributes["name"]; attr == nil {
		t.Error("expected a diff entry for `name`")
	} else if attr.New != "Tracked By Terraform" || attr.Old != "Console Edit" {
		t.Errorf("got name diff %+v, want Old=Console Edit New=Tracked By Terraform", attr)
	}
}

func TestResourceProjectSchema(t *testing.T) {
	r := resourceProject()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}

	identifier := r.Schema["identifier"]
	if identifier == nil {
		t.Fatal("expected identifier attribute in schema")
	}
	if !identifier.Required || !identifier.ForceNew {
		t.Errorf("identifier should be Required+ForceNew, got Required=%v ForceNew=%v", identifier.Required, identifier.ForceNew)
	}
	if identifier.ValidateFunc == nil {
		t.Error("identifier should have a ValidateFunc enforcing the regex")
	}

	// name and description must be Optional+Computed so terraform's diff resolver
	// keeps the state value when config is null — that's what makes console edits
	// "stick" instead of getting reverted on the next apply.
	for _, field := range []string{"name", "description"} {
		s := r.Schema[field]
		if s == nil {
			t.Fatalf("expected %s attribute in schema", field)
		}
		if s.Required {
			t.Errorf("%s should not be Required", field)
		}
		if !s.Optional || !s.Computed {
			t.Errorf("%s should be Optional+Computed (got Optional=%v Computed=%v) so unspecified-in-config drift is ignored", field, s.Optional, s.Computed)
		}
	}

	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required (drift always surfaces)")
	}

	// terraform's auto-managed `id` should NOT appear in the schema map.
	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}

func TestResourceProjectIdentifierValidation(t *testing.T) {
	identifier := resourceProject().Schema["identifier"]
	if identifier.ValidateFunc == nil {
		t.Fatal("identifier has no ValidateFunc")
	}

	cases := []struct {
		value string
		valid bool
	}{
		{"ecomm", true},
		{"ec0mm", true},
		{"a", true},                      // single char OK
		{"twentycharacterident", true},   // exactly 20 chars
		{"twentyonecharidentifier", false}, // > 20
		{"", false},                      // empty
		{"Ecomm", false},                 // uppercase
		{"ec-omm", false},                // hyphen
		{"ec omm", false},                // space
		{"ec_omm", false},                // underscore
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			_, errs := identifier.ValidateFunc(tc.value, "identifier")
			if tc.valid && len(errs) > 0 {
				t.Errorf("expected %q to be valid, got errors: %v", tc.value, errs)
			}
			if !tc.valid && len(errs) == 0 {
				t.Errorf("expected %q to be rejected, got no errors", tc.value)
			}
		})
	}
}
