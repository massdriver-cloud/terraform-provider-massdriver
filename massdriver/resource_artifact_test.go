package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

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
		artifact = jsonencode({data={foo="bar"},specs={bam="bizzle"}})
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
