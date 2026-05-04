package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// Both Create and Update on massdriver_artifact must hard-error with a
// message that points users at the replacement resources. Pinning this
// behavior so a future refactor that re-enables either path is a deliberate
// decision, not a regression.
//
// SDK 2.16's CreateContextFunc and UpdateContextFunc are distinct types with
// the same signature; calling them through a shared helper keeps the test
// table-driven without fighting the type system.
func TestResourceArtifactWritesDisabled(t *testing.T) {
	r := resourceArtifact()
	cases := map[string]func(context.Context, *schema.ResourceData, any) diag.Diagnostics{
		"Create": r.CreateContext,
		"Update": r.UpdateContext,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
				"field":    "vpc",
				"name":     "VPC",
				"artifact": `{}`,
			})
			diags := fn(t.Context(), rd, nil)
			if !diags.HasError() {
				t.Fatalf("expected %s to error, got none", name)
			}
			summary := diags[0].Summary
			if !strings.Contains(summary, "no longer supports") {
				t.Errorf("error %q should explain that the operation is disabled", summary)
			}
			if !strings.Contains(summary, "massdriver_resource") || !strings.Contains(summary, "massdriver_imported_resource") {
				t.Errorf("error %q should point users at both replacement resources", summary)
			}
		})
	}
}

// Read and Delete must remain wired so users with existing state can refresh
// and migrate off cleanly.
func TestResourceArtifactReadAndDeleteStillWired(t *testing.T) {
	r := resourceArtifact()
	if r.ReadContext == nil {
		t.Error("ReadContext should remain wired so refresh keeps working")
	}
	if r.DeleteContext == nil {
		t.Error("DeleteContext should remain wired so users can clean up")
	}
}

func TestAccMassdriverArtifactBasic(t *testing.T) {
	// massdriver_artifact no longer accepts create/update — see
	// resourceArtifactWritesDisabled. The acceptance test needed a live
	// Create, which now hard-errors, so skip it. Refresh/Delete-only paths
	// are exercised through users' existing state and don't need acceptance
	// coverage here.
	t.Skip("massdriver_artifact is frozen for new writes; use massdriver_resource or massdriver_imported_resource")

	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverArtifactConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverArtifactExists("massdriver_artifact.new"),
				),
			},
		},
	})
}

func testAccCheckMassdriverArtifactConfigBasic() string {
	return `
	resource "massdriver_artifact" "new" {
		field = "example-artifact"
		provider_resource_id = "arn:::something"
		type = "type"
		schema_path = "testdata/schema-artifacts.json"
		specification_path = "testdata/massdriver.yaml"
		name = "name"
		artifact = jsonencode({foo="bar",bam="bizzle"})
	}
	`
}

func testAccCheckMassdriverArtifactExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]

		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID set")
		}

		return nil
	}
}
