package massdriver

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccMassdriverPackageAlarmBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t) },
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccCheckMassdriverPackageAlarmConfigBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverPackageAlarmExists("massdriver_package_alarm.new"),
				),
			},
			{
				Config: testAccCheckMassdriverPackageAlarmConfigSlim(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckMassdriverPackageAlarmExists("massdriver_package_alarm.new"),
				),
			},
		},
	})
}

func testAccCheckMassdriverPackageAlarmConfigBasic() string {
	return `
	resource "massdriver_package_alarm" "new" {
		cloud_resource_id = "arn:::something"
		display_name = "CPU alarm"
		metric {
			name = "Metric Name"
			namespace = "Metric/Namespace"
			statistic = "SUM"
			dimensions = {
				"foo" = "bar"
			}
		}
		threshold = 80.0
		period_minutes = 5
		comparison_operator = "GreaterThanThreshold"
	}
	`
}

func testAccCheckMassdriverPackageAlarmConfigSlim() string {
	return `
	resource "massdriver_package_alarm" "new" {
		cloud_resource_id = "arn:::something"
		display_name = "CPU alarm"
		metric {
			name = "Metric Name"
			namespace = "Metric/Namespace"
		}
	}
	`
}

func testAccCheckMassdriverPackageAlarmExists(n string) resource.TestCheckFunc {
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
