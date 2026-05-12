package massdriver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// All four CRUD contexts must remain wired — artifact stays functional in
// v1.3 because the REST endpoint redirects to `/resources`. The deprecation
// is communicated via DeprecationMessage, not via a hard-error on writes.
func TestResourceArtifactAllContextsWired(t *testing.T) {
	r := resourceArtifact()
	if r.CreateContext == nil {
		t.Error("CreateContext should be wired")
	}
	if r.ReadContext == nil {
		t.Error("ReadContext should be wired")
	}
	if r.UpdateContext == nil {
		t.Error("UpdateContext should be wired")
	}
	if r.DeleteContext == nil {
		t.Error("DeleteContext should be wired")
	}
	if r.DeprecationMessage == "" {
		t.Error("DeprecationMessage should be set on the deprecated resource")
	}
	if !strings.Contains(r.DeprecationMessage, "massdriver_resource") {
		t.Errorf("DeprecationMessage should point at massdriver_resource; got %q", r.DeprecationMessage)
	}
}

func TestAccMassdriverArtifactBasic(t *testing.T) {
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
