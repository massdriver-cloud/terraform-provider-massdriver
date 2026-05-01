package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"terraform-provider-massdriver/internal/gqlmock"
)

// Component IDs use `*` as the project/identifier separator (e.g., `ecomm*db`)
// rather than the `-` used elsewhere.

func TestResourceComponentCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"addComponent": {
			"data": map[string]any{
				"addComponent": map[string]any{
					"result": map[string]any{
						"id":   "ecomm*db",
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
									"id":   "ecomm*db",
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

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":    "db",
		"project_id":    "ecomm",
		"name":          "Primary Database",
		"oci_repo_name": "aws-rds-cluster",
	})

	diags := resourceComponentCreate(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ecomm*db" {
		t.Errorf("got id %q, want ecomm*db", rd.Id())
	}
	if rd.Get("identifier").(string) != "db" {
		t.Errorf("got identifier %q, want db (recovered from `<project>*<identifier>`)", rd.Get("identifier"))
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
	rd.SetId("ecomm*db")

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
									"id":   "ecomm*db",
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

	// Simulates `terraform import` — only the platform ID is set, project_id must be recovered by splitting on `*`.
	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm*db")

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
	if rd.Get("oci_repo_name").(string) != "aws-rds-cluster" {
		t.Errorf("got oci_repo_name %q, want aws-rds-cluster", rd.Get("oci_repo_name"))
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

func TestResourceComponentDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"removeComponent": {
			"data": map[string]any{
				"removeComponent": map[string]any{
					"result":     map[string]any{"id": "ecomm*db", "name": "db"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{})
	rd.SetId("ecomm*db")

	diags := resourceComponentDelete(t.Context(), rd, pc)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("removeComponent")); vars["id"] != "ecomm*db" {
		t.Errorf("got id %v, want ecomm*db", vars["id"])
	}
}

func TestResourceComponentCreateDefaultsNameToIdentifier(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"addComponent": {
			"data": map[string]any{
				"addComponent": map[string]any{
					"result":     map[string]any{"id": "ecomm*db", "name": "db"},
					"successful": true,
				},
			},
		},
		"listComponents": {
			"data": map[string]any{
				"project": map[string]any{
					"blueprint": map[string]any{
						"components": map[string]any{
							"items": []map[string]any{{"id": "ecomm*db", "name": "db"}},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceComponent().Schema, map[string]any{
		"identifier":    "db",
		"project_id":    "ecomm",
		"oci_repo_name": "aws-rds-cluster",
	})

	if diags := resourceComponentCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("addComponent"))["input"].(map[string]any)
	if input["name"] != "db" {
		t.Errorf("got input.name %v, want db (defaulted from identifier)", input["name"])
	}
}

func TestResourceComponentIgnoresDriftWhenConfigUnset(t *testing.T) {
	r := resourceComponent()

	state := &terraform.InstanceState{
		ID: "ecomm*db",
		Attributes: map[string]string{
			"id":            "ecomm*db",
			"identifier":    "db",
			"project_id":    "ecomm",
			"oci_repo_name": "aws-rds-cluster",
			"name":          "Primary Database (manual edit)",
			"description":   "edited in the console",
		},
	}
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"identifier":    "db",
		"project_id":    "ecomm",
		"oci_repo_name": "aws-rds-cluster",
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

func TestResourceComponentSchema(t *testing.T) {
	r := resourceComponent()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if r.UpdateContext != nil {
		t.Error("UpdateContext should be nil — component has no update mutation")
	}
	// Every user-facing input is ForceNew because there's no update mutation.
	for _, field := range []string{"name", "oci_repo_name", "description", "identifier"} {
		if !r.Schema[field].ForceNew {
			t.Errorf("%s should be ForceNew (no update path)", field)
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
	if _, present := r.Schema["id"]; present {
		t.Error("id should not be defined in schema — terraform manages it automatically")
	}
}
