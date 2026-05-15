package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMassdriverArtifactDataSourceBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverArtifactDataSourceConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverArtifactDataSourceExists("data.massdriver_artifact.test"),
					resource.TestCheckResourceAttrSet("data.massdriver_artifact.test", "name"),
					resource.TestCheckResourceAttrSet("data.massdriver_artifact.test", "type"),
					resource.TestCheckResourceAttrSet("data.massdriver_artifact.test", "payload"),
				),
			},
		},
	})
}

func testAccCheckMassdriverArtifactDataSourceConfigBasic() string {
	// Note: This test requires an existing artifact in the test environment
	// The artifact ID should use the dot-separated format: project-env-manifest.field
	return `
	data "massdriver_artifact" "test" {
		id = "test-env-pkg.test-artifact"
	}
	`
}

func testAccCheckMassdriverArtifactDataSourceExists(n string) resource.TestCheckFunc {
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
