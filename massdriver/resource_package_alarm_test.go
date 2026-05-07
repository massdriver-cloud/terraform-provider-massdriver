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

// Read pulls from the instance_alarm GraphQL endpoint now that the package
// alarm REST endpoint has been removed. Verifies the field mapping including
// the seconds→minutes conversion on `period_minutes` and the dimensions
// list→map conversion on the metric block.
func TestResourcePackageAlarmReadViaGraphQL(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":                 "alarm-uuid",
					"displayName":        "RDS High CPU",
					"cloudResourceId":    "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
					"comparisonOperator": "GreaterThanThreshold",
					"threshold":          80.0,
					"period":             300, // seconds — should map to period_minutes=5
					"metric": map[string]any{
						"namespace": "AWS/RDS",
						"name":      "CPUUtilization",
						"statistic": "Average",
						"dimensions": []map[string]any{
							{"name": "DBInstanceIdentifier", "value": "prod-db"},
						},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("display_name").(string) != "RDS High CPU" {
		t.Errorf("got display_name %q", rd.Get("display_name"))
	}
	if rd.Get("cloud_resource_id").(string) != "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu" {
		t.Errorf("got cloud_resource_id %q", rd.Get("cloud_resource_id"))
	}
	if rd.Get("threshold").(float64) != 80.0 {
		t.Errorf("got threshold %v, want 80.0", rd.Get("threshold"))
	}
	// period_minutes is the load-bearing seconds→minutes conversion.
	if got := rd.Get("period_minutes").(int); got != 5 {
		t.Errorf("got period_minutes %v, want 5 (300 seconds / 60)", got)
	}
	if rd.Get("comparison_operator").(string) != "GreaterThanThreshold" {
		t.Errorf("got comparison_operator %q", rd.Get("comparison_operator"))
	}

	metric := rd.Get("metric").([]any)
	if len(metric) != 1 {
		t.Fatalf("got %d metric blocks, want 1", len(metric))
	}
	m := metric[0].(map[string]any)
	if m["namespace"] != "AWS/RDS" || m["name"] != "CPUUtilization" {
		t.Errorf("got metric %+v", m)
	}
	dims := m["dimensions"].(map[string]any)
	if dims["DBInstanceIdentifier"] != "prod-db" {
		t.Errorf("got dimensions %v, want DBInstanceIdentifier=prod-db", dims)
	}
}

// "not found" from the GraphQL endpoint must clear state — terraform then
// plans a recreate which hard-errors via WritesDisabled, prompting the user
// to migrate to massdriver_instance_alarm.
func TestResourcePackageAlarmReadClearsOnNotFound(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data":   map[string]any{"instanceAlarm": nil},
			"errors": []map[string]any{{"message": "not found"}},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found, got %q", rd.Id())
	}
}

// Pre-modern timestamp-format IDs predate UUIDs; the GraphQL endpoint can't
// look them up. Read must short-circuit (leave state) so a refresh against an
// ancient state file doesn't fail loudly.
func TestResourcePackageAlarmReadShortCircuitsLegacyTimestampID(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{}) // no getInstanceAlarm response — must not be called

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"display_name":      "stale",
		"cloud_resource_id": "arn:::x",
	})
	rd.SetId("2021-04-15T12:00:00Z") // RFC3339 timestamp ID from the pre-UUID era

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(rec.Requests) != 0 {
		t.Errorf("legacy timestamp IDs should skip the API call entirely, got %d requests", len(rec.Requests))
	}
	if rd.Id() != "2021-04-15T12:00:00Z" {
		t.Errorf("ID should be untouched on the legacy short-circuit, got %q", rd.Id())
	}
}

func TestResourcePackageAlarmDeleteViaGraphQL(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteInstanceAlarm": {
			"data": map[string]any{
				"deleteInstanceAlarm": map[string]any{
					"result":     map[string]any{"id": "alarm-uuid", "displayName": "RDS High CPU"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared after delete, got %q", rd.Id())
	}
	if vars := rec.FindRequest("deleteInstanceAlarm"); vars == nil {
		t.Error("deleteInstanceAlarm was not called")
	}
}

// Legacy timestamp IDs can't be deleted server-side (the REST endpoint that
// understood them is gone). Delete should silently clear state so destroy
// works on ancient state files.
// suppressAllDiffs is the load-bearing piece that keeps terraform apply from
// failing on existing deployments after the package alarm Create/Update
// endpoints went away. Even with substantial drift between HCL and state,
// terraform must plan zero changes — otherwise it would call Update, which
// hard-errors via WritesDisabled and forces users to migrate before they can
// apply *any* unrelated change in their configuration.
func TestResourcePackageAlarmSuppressesAllDriftFromUpdates(t *testing.T) {
	r := resourcePackageAlarm()

	// Existing state — what a user accumulated under the live REST endpoint.
	state := &terraform.InstanceState{
		ID: "alarm-uuid",
		Attributes: map[string]string{
			"id":                       "alarm-uuid",
			"cloud_resource_id":        "arn:aws:cloudwatch:us-east-1:111:alarm/old-arn",
			"display_name":             "Old Name",
			"threshold":                "50",
			"period_minutes":           "5",
			"comparison_operator":      "GreaterThanThreshold",
			"package_id":               "ecomm-prod-db",
			"metric.#":                 "1",
			"metric.0.name":            "OldMetric",
			"metric.0.namespace":       "AWS/RDS",
			"metric.0.statistic":       "Average",
			"metric.0.dimensions.%":    "1",
			"metric.0.dimensions.foo":  "bar",
		},
	}

	// Brand-new HCL — every single field has changed from state.
	cfg := terraform.NewResourceConfigRaw(map[string]any{
		"cloud_resource_id":   "arn:aws:cloudwatch:us-east-1:111:alarm/new-arn",
		"display_name":        "New Name",
		"threshold":           99.0,
		"period_minutes":      10,
		"comparison_operator": "LessThanThreshold",
		"package_id":          "ecomm-prod-db",
		"metric": []any{map[string]any{
			"name":      "NewMetric",
			"namespace": "AWS/EC2",
			"statistic": "Sum",
			"dimensions": map[string]any{
				"baz": "qux",
			},
		}},
	})

	diff, err := r.Diff(t.Context(), state, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected diff error: %v", err)
	}
	// `nil` diff or an empty one both mean "no changes planned" — either is
	// acceptable, terraform won't call Update in either case.
	if diff != nil && !diff.Empty() {
		t.Errorf("expected zero-change diff regardless of drift, got %d attribute changes:", len(diff.Attributes))
		for k, attr := range diff.Attributes {
			t.Errorf("  %s: %+v", k, attr)
		}
	}
}

func TestResourcePackageAlarmDeleteShortCircuitsLegacyTimestampID(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::x",
	})
	rd.SetId("2021-04-15T12:00:00Z")

	if diags := resourcePackageAlarmDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared even for legacy IDs, got %q", rd.Id())
	}
	if len(rec.Requests) != 0 {
		t.Errorf("legacy timestamp IDs should skip the API call, got %d requests", len(rec.Requests))
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
