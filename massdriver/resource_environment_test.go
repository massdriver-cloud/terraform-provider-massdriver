package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMassdriverEnvironmentBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverEnvironmentConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverEnvironmentExists("massdriver_environment.test"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "name", "Test Environment"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "slug", "test"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "description", "A test environment"),
				),
			},
		},
	})
}

func TestAccMassdriverEnvironmentUpdate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverEnvironmentConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverEnvironmentExists("massdriver_environment.test"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "name", "Test Environment"),
				),
			},
			{
				Config: testAccCheckMassdriverEnvironmentConfigUpdate(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverEnvironmentExists("massdriver_environment.test"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "name", "Updated Test Environment"),
					resource.TestCheckResourceAttr("massdriver_environment.test", "description", "An updated test environment"),
				),
			},
		},
	})
}

func testAccCheckMassdriverEnvironmentConfigBasic() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Test Project for Env"
		slug        = "test-project-env"
		description = "A test project for environment tests"
	}

	resource "massdriver_environment" "test" {
		project_id  = massdriver_project.test.id
		name        = "Test Environment"
		slug        = "test"
		description = "A test environment"
	}
	`
}

func testAccCheckMassdriverEnvironmentConfigUpdate() string {
	return `
	resource "massdriver_project" "test" {
		name        = "Test Project for Env"
		slug        = "test-project-env"
		description = "A test project for environment tests"
	}

	resource "massdriver_environment" "test" {
		project_id  = massdriver_project.test.id
		name        = "Updated Test Environment"
		slug        = "test"
		description = "An updated test environment"
	}
	`
}

func testAccCheckMassdriverEnvironmentExists(n string) resource.TestCheckFunc {
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
