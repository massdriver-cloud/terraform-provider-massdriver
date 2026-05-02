package massdriver

import (
	"testing"

	"terraform-provider-massdriver/internal/gqlmock"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestResourceComponentCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"addComponent": {
			"data": map[string]any{
				"addComponent": map[string]any{
					"result": map[string]any{
						"id":   "ecomm-db",
						"name": "Primary Database",
						"ociRepo": map[string]any{
							"id":   "repo-1",
							"name": "aws-rds-cluster",
						},
					},
					"successful": true,
				},
			},
		},
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{
								{
									"id":         "ecomm-db",
									"name":       "Primary Database",
									"attributes": map[string]any{"team": "platform"},
									"ociRepo": map[string]any{
										"id":   "repo-1",
										"name": "aws-rds-cluster",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"name":        "Primary Database",
		"bundle_name": "aws-rds-cluster",
		"attributes":  map[string]any{"team": "platform"},
	})

	diags := resourceComponentCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm-db" {
		t.Errorf("got id %q, want ecomm-db", rd.Id())
	}

	addReq := rec.FindRequest("addComponent")
	if addReq == nil {
		t.Fatal("addComponent was not called")
	}
	vars := gqlmock.Variables(addReq)
	if vars["projectId"] != "ecomm" {
		t.Errorf("got projectId %v, want ecomm", vars["projectId"])
	}
	if vars["ociRepoName"] != "aws-rds-cluster" {
		t.Errorf("got ociRepoName %v, want aws-rds-cluster", vars["ociRepoName"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["id"] != "db" {
		t.Errorf("got input.id %v, want db (the bare identifier)", input["id"])
	}
	if input["name"] != "Primary Database" {
		t.Errorf("got input.name %v, want Primary Database", input["name"])
	}
	// `attributes` is the JSON scalar that double-encodes; the wire payload is the JSON-encoded string.
	if input["attributes"] != `{"team":"platform"}` {
		t.Errorf("got input.attributes %v, want JSON-encoded team=platform", input["attributes"])
	}

	// Read populates attributes back into state from the API response.
	if got := rd.Get("attributes").(map[string]any); got["team"] != "platform" {
		t.Errorf("got attributes %v after Read, wanted team=platform", got)
	}
}

func TestResourceComponentReadDrops404(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"project_id": "ecomm",
	})
	rd.SetId("ecomm-db")

	diags := resourceComponentRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared when component vanishes server-side, got %q", rd.Id())
	}
}

func TestResourceComponentReadRecoversProjectIDOnImport(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{
								{
									"id":   "ecomm-db",
									"name": "Primary Database",
									"ociRepo": map[string]any{
										"id":   "repo-1",
										"name": "aws-rds-cluster",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// Simulates `terraform import` — only the platform ID is set, project_id must be recovered.
	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm-db")

	diags := resourceComponentRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("project_id").(string) != "ecomm" {
		t.Errorf("got project_id %q, want ecomm", rd.Get("project_id"))
	}
	if rd.Get("identifier").(string) != "db" {
		t.Errorf("got identifier %q, want db", rd.Get("identifier"))
	}
	if rd.Get("bundle_name").(string) != "aws-rds-cluster" {
		t.Errorf("got bundle_name %q, want aws-rds-cluster", rd.Get("bundle_name"))
	}

	listReq := rec.FindRequest("listComponents")
	if listReq == nil {
		t.Fatal("listComponents was not called")
	}
	vars := gqlmock.Variables(listReq)
	if vars["projectId"] != "ecomm" {
		t.Errorf("got projectId %v, want ecomm", vars["projectId"])
	}
}

func TestResourceComponentUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateComponent": {
			"data": map[string]any{
				"updateComponent": map[string]any{
					"result": map[string]any{
						"id":          "ecomm-db",
						"name":        "Renamed",
						"description": "updated",
						"attributes":  map[string]any{"team": "infra"},
					},
					"successful": true,
				},
			},
		},
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{
								{
									"id":          "ecomm-db",
									"name":        "Renamed",
									"description": "updated",
									"attributes":  map[string]any{"team": "infra"},
									"ociRepo": map[string]any{
										"id":   "repo-1",
										"name": "aws-rds-cluster",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"name":        "Renamed",
		"description": "updated",
		"bundle_name": "aws-rds-cluster",
		"attributes":  map[string]any{"team": "infra"},
	})
	rd.SetId("ecomm-db")

	if diags := resourceComponentUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateComponent")
	if updateReq == nil {
		t.Fatal("updateComponent was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "ecomm-db" {
		t.Errorf("got id %v, want ecomm-db", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Renamed" {
		t.Errorf("got input.name %v, want Renamed", input["name"])
	}
	// The double-encoded JSON-scalar value should reflect the new attributes.
	if input["attributes"] != `{"team":"infra"}` {
		t.Errorf("got input.attributes %v, want JSON-encoded team=infra", input["attributes"])
	}
}

func TestResourceComponentDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"removeComponent": {
			"data": map[string]any{
				"removeComponent": map[string]any{
					"result":     map[string]any{"id": "ecomm-db", "name": "db"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm-db")

	diags := resourceComponentDelete(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("removeComponent")); vars["id"] != "ecomm-db" {
		t.Errorf("got id %v, want ecomm-db", vars["id"])
	}
}

func TestResourceComponentCreateDefaultsNameToIdentifier(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"addComponent": {
			"data": map[string]any{
				"addComponent": map[string]any{
					"result":     map[string]any{"id": "ecomm-db", "name": "db"},
					"successful": true,
				},
			},
		},
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{{"id": "ecomm-db", "name": "db"}},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
	})

	if diags := resourceComponentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("addComponent"))["input"].(map[string]any)
	if input["name"] != "db" {
		t.Errorf("got input.name %v, want db (defaulted from identifier)", input["name"])
	}
}

// Drift on name/description (Optional+Computed) must NOT show up as a plan
// diff when the user omits those fields from config.
func TestResourceComponentIgnoresNameDriftWhenConfigUnset(t *testing.T) {
	r := resourceComponent()

	state := &terraform.InstanceState{
		ID: "ecomm-db",
		Attributes: map[string]string{
			"id":             "ecomm-db",
			"identifier":     "db",
			"project_id":     "ecomm",
			"bundle_name":    "aws-rds-cluster",
			"name":           "Primary Database (manual edit)",
			"description":    "edited in the console",
			"attributes.%":   "1",
			"attributes.team": "platform",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		"attributes":  map[string]any{"team": "platform"},
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

// Conversely, drift on `attributes` MUST surface — these are how permissions
// are calculated, so silent drift would be a security issue.
func TestResourceComponentSurfacesAttributesDrift(t *testing.T) {
	r := resourceComponent()

	state := &terraform.InstanceState{
		ID: "ecomm-db",
		Attributes: map[string]string{
			"id":              "ecomm-db",
			"identifier":      "db",
			"project_id":      "ecomm",
			"bundle_name":     "aws-rds-cluster",
			"attributes.%":    "1",
			"attributes.team": "infra", // drifted via console edit
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier":  "db",
		"project_id":  "ecomm",
		"bundle_name": "aws-rds-cluster",
		"attributes":  map[string]any{"team": "platform"},
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

func TestResourceComponentSchema(t *testing.T) {
	r := resourceComponent()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if r.UpdateContext == nil {
		t.Error("UpdateContext should be set — updateComponent mutation now exists")
	}
	// Mutable fields are not ForceNew now that updateComponent exists.
	for _, field := range []string{"name", "description", "attributes"} {
		if r.Schema[field].ForceNew {
			t.Errorf("%s should NOT be ForceNew — updateComponent supports changing it", field)
		}
	}
	// Identity fields stay ForceNew.
	for _, field := range []string{"identifier", "project_id", "bundle_name"} {
		if !r.Schema[field].ForceNew {
			t.Errorf("%s should be ForceNew", field)
		}
	}
	if id := r.Schema["identifier"]; id == nil || !id.Required || id.ValidateFunc == nil {
		t.Error("identifier should be Required with a ValidateFunc")
	}
	for _, field := range []string{"name", "description"} {
		s := r.Schema[field]
		if s == nil || s.Required || !s.Optional || !s.Computed {
			t.Errorf("%s should be Optional+Computed (got Required=%v Optional=%v Computed=%v)", field, s.Required, s.Optional, s.Computed)
		}
	}
	if attrs := r.Schema["attributes"]; attrs == nil || !attrs.Required {
		t.Error("attributes should be Required (drift always surfaces)")
	}
	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}
