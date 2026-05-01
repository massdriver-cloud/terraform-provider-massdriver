package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestResourceEnvironmentCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createEnvironment": {
			"data": map[string]any{
				"createEnvironment": map[string]any{
					"result": map[string]any{
						"id":          "ecomm-prod",
						"name":        "Production",
						"description": "live env",
					},
					"successful": true,
				},
			},
		},
		"getEnvironment": {
			"data": map[string]any{
				"environment": map[string]any{
					"id":          "ecomm-prod",
					"name":        "Production",
					"description": "live env",
					"project": map[string]any{
						"id":   "ecomm",
						"name": "Ecomm",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier":  "prod",
		"project_id":  "ecomm",
		"name":        "Production",
		"description": "live env",
	})

	diags := resourceEnvironmentCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm-prod" {
		t.Errorf("got id %q, want ecomm-prod", rd.Id())
	}
	if rd.Get("identifier").(string) != "prod" {
		t.Errorf("got identifier %q, want prod (recovered from `<project>-<identifier>`)", rd.Get("identifier"))
	}
	if rd.Get("project_id").(string) != "ecomm" {
		t.Errorf("got project_id %q, want ecomm", rd.Get("project_id"))
	}

	createReq := rec.FindRequest("createEnvironment")
	if createReq == nil {
		t.Fatal("createEnvironment was not called")
	}
	vars := gqlmock.Variables(createReq)
	if vars["projectId"] != "ecomm" {
		t.Errorf("got projectId %v, want ecomm", vars["projectId"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["id"] != "prod" {
		t.Errorf("got input.id %v, want prod (just the identifier, not the composed platform ID)", input["id"])
	}
	if input["name"] != "Production" {
		t.Errorf("got input.name %v, want Production", input["name"])
	}
}

func TestResourceEnvironmentRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getEnvironment": {
			"data": map[string]any{
				"environment": map[string]any{
					"id":          "ecomm-prod",
					"name":        "Production",
					"description": "live env",
					"project": map[string]any{
						"id":   "ecomm",
						"name": "Ecomm",
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("ecomm-prod")

	diags := resourceEnvironmentRead(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("name").(string) != "Production" {
		t.Errorf("got name %q, want Production", rd.Get("name"))
	}
	if rd.Get("project_id").(string) != "ecomm" {
		t.Errorf("got project_id %q, want ecomm", rd.Get("project_id"))
	}
	if rd.Get("identifier").(string) != "prod" {
		t.Errorf("got identifier %q, want prod (the suffix after the project prefix)", rd.Get("identifier"))
	}
}

func TestResourceEnvironmentUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateEnvironment": {
			"data": map[string]any{
				"updateEnvironment": map[string]any{
					"result": map[string]any{
						"id":          "ecomm-prod",
						"name":        "Renamed",
						"description": "updated",
					},
					"successful": true,
				},
			},
		},
		"getEnvironment": {
			"data": map[string]any{
				"environment": map[string]any{
					"id":          "ecomm-prod",
					"name":        "Renamed",
					"description": "updated",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier":  "prod",
		"project_id":  "ecomm",
		"name":        "Renamed",
		"description": "updated",
	})
	rd.SetId("ecomm-prod")

	diags := resourceEnvironmentUpdate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateEnvironment")
	if updateReq == nil {
		t.Fatal("updateEnvironment was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "ecomm-prod" {
		t.Errorf("got id %v, want ecomm-prod", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["name"] != "Renamed" {
		t.Errorf("got input.name %v, want Renamed", input["name"])
	}
}

func TestResourceEnvironmentDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteEnvironment": {
			"data": map[string]any{
				"deleteEnvironment": map[string]any{
					"result":     map[string]any{"id": "ecomm-prod", "name": "Production"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{})
	rd.SetId("ecomm-prod")

	diags := resourceEnvironmentDelete(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deleteEnvironment")); vars["id"] != "ecomm-prod" {
		t.Errorf("got id %v, want ecomm-prod", vars["id"])
	}
}

func TestResourceEnvironmentCreateDefaultsNameToIdentifier(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createEnvironment": {
			"data": map[string]any{
				"createEnvironment": map[string]any{
					"result":     map[string]any{"id": "ecomm-prod", "name": "prod"},
					"successful": true,
				},
			},
		},
		"getEnvironment": {
			"data": map[string]any{
				"environment": map[string]any{"id": "ecomm-prod", "name": "prod"},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceEnvironment().Schema, map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
	})

	if diags := resourceEnvironmentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createEnvironment"))["input"].(map[string]any)
	if input["name"] != "prod" {
		t.Errorf("got input.name %v, want prod (defaulted from identifier)", input["name"])
	}
}

func TestResourceEnvironmentIgnoresDriftWhenConfigUnset(t *testing.T) {
	r := resourceEnvironment()

	state := &terraform.InstanceState{
		ID: "ecomm-prod",
		Attributes: map[string]string{
			"id":          "ecomm-prod",
			"identifier":  "prod",
			"project_id":  "ecomm",
			"name":        "Production (manual edit)",
			"description": "edited in the console",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier": "prod",
		"project_id": "ecomm",
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

func TestResourceEnvironmentSchema(t *testing.T) {
	r := resourceEnvironment()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if id := r.Schema["identifier"]; id == nil || !id.Required || !id.ForceNew || id.ValidateFunc == nil {
		t.Error("identifier should be Required+ForceNew with a ValidateFunc")
	}
	if pid := r.Schema["project_id"]; pid == nil || !pid.Required || !pid.ForceNew {
		t.Error("project_id should be Required+ForceNew")
	}
	for _, field := range []string{"name", "description"} {
		s := r.Schema[field]
		if s == nil || s.Required || !s.Optional || !s.Computed {
			t.Errorf("%s should be Optional+Computed (got Required=%v Optional=%v Computed=%v)", field, s.Required, s.Optional, s.Computed)
		}
	}
	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}
