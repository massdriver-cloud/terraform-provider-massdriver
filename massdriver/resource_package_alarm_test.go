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

// Both Create and Update on massdriver_package_alarm must hard-error and
// point users at massdriver_instance_alarm. Mirrors the artifact-side test.
func TestResourcePackageAlarmWritesDisabled(t *testing.T) {
	r := resourcePackageAlarm()
	cases := map[string]func(context.Context, *schema.ResourceData, any) diag.Diagnostics{
		"Create": r.CreateContext,
		"Update": r.UpdateContext,
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			rd := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
				"cloud_resource_id": "arn:::x",
				"display_name":      "x",
			})
			diags := fn(t.Context(), rd, nil)
			if !diags.HasError() {
				t.Fatalf("expected %s to error, got none", name)
			}
			summary := diags[0].Summary
			if !strings.Contains(summary, "no longer supports") {
				t.Errorf("error %q should explain the operation is disabled", summary)
			}
			if !strings.Contains(summary, "massdriver_instance_alarm") {
				t.Errorf("error %q should point users at massdriver_instance_alarm", summary)
			}
		})
	}
}

// Read and Delete must remain wired so users with existing state can refresh
// and migrate off cleanly.
func TestResourcePackageAlarmReadAndDeleteStillWired(t *testing.T) {
	r := resourcePackageAlarm()
	if r.ReadContext == nil {
		t.Error("ReadContext should remain wired so refresh keeps working")
	}
	if r.DeleteContext == nil {
		t.Error("DeleteContext should remain wired so users can clean up")
	}
}

func TestAccMassdriverPackageAlarmBasic(t *testing.T) {
	// massdriver_package_alarm no longer accepts create/update — see
	// resourcePackageAlarmWritesDisabled. The acceptance test needed a live
	// Create, which now hard-errors, so skip it. Refresh/Delete-only paths
	// are exercised through users' existing state and don't need acceptance
	// coverage here.
	t.Skip("massdriver_package_alarm is frozen for new writes; use massdriver_instance_alarm")

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
