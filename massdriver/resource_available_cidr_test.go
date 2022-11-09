package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMassdriverAvailableCidrBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverAvailableCidrConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverAvailableCidrExists("massdriver_available_cidr.new"),
				),
			},
		},
	})
}

func testAccCheckMassdriverAvailableCidrConfigBasic() string {
	return `
	resource "massdriver_available_cidr" "new" {
		parent_cidrs = ["10.0.0.0/16"]
		used_cidrs = ["10.0.0.0/24"]
		mask = 24
	}
	`
}

func testAccCheckMassdriverAvailableCidrExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]

		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID set")
		}

		if rs.Primary.ID != "10.0.1.0/24" {
			return fmt.Errorf("Invalid resultant CIDR")
		}

		return nil
	}
}
