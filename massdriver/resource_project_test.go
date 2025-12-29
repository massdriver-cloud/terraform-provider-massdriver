package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMassdriverProjectBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverProjectConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Test Project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "slug", "test-project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "description", "A test project"),
				),
			},
		},
	})
}

func TestAccMassdriverProjectUpdate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverProjectConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Test Project"),
				),
			},
			{
				Config: testAccCheckMassdriverProjectConfigUpdate(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverProjectExists("massdriver_project.test"),
					resource.TestCheckResourceAttr("massdriver_project.test", "name", "Updated Test Project"),
					resource.TestCheckResourceAttr("massdriver_project.test", "description", "An updated test project"),
				),
			},
		},
	})
}

func testAccCheckMassdriverProjectConfigBasic() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Test Project"
		slug        = "test-project"
		description = "A test project"
	}
	`
}

func testAccCheckMassdriverProjectConfigUpdate() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Updated Test Project"
		slug        = "test-project"
		description = "An updated test project"
	}
	`
}

func testAccCheckMassdriverProjectExists(n string) resource.TestCheckFunc {
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
