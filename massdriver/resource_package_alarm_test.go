package massdriver

import (
	"fmt"
	"strings"
	"testing"

	"terraform-provider-massdriver/internal/gqlmock"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// The self-heal Create path: when state has been corrupted by pre-1.3
// deploys (which 404'd against the dead REST endpoint and cleared the
// alarm's UUID from state), Create should look the alarm up by
// instance + cloud_resource_id and adopt it back into state. No
// createInstanceAlarm call should fire — we use the existing record.
func TestResourcePackageAlarmCreateAdoptsOrphanedAlarm(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listInstanceAlarms": {
			"data": map[string]any{
				"instanceAlarms": map[string]any{
					"cursor": map[string]any{"next": ""},
					"items": []map[string]any{
						{
							"id":              "wrong-alarm",
							"cloudResourceId": "arn:::different",
						},
						{
							"id":              "recovered-uuid",
							"displayName":     "RDS High CPU",
							"cloudResourceId": "arn:::target",
						},
					},
				},
			},
		},
		// Read fires after Create succeeds — needs to return the recovered alarm.
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "recovered-uuid",
					"displayName":     "RDS High CPU",
					"cloudResourceId": "arn:::target",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::target",
		"display_name":      "RDS High CPU",
		"package_id":        "ecomm-prod-db",
	})

	if diags := resourcePackageAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "recovered-uuid" {
		t.Errorf("expected ID adopted from server lookup; got %q want recovered-uuid", rd.Id())
	}
	if rec.FindRequest("createInstanceAlarm") != nil {
		t.Error("Create must not fire when the self-heal lookup adopts an existing alarm")
	}
}

// User genuinely adding a new alarm — server has nothing to recover. Create
// must fall through and call createInstanceAlarm via the GraphQL endpoint.
func TestResourcePackageAlarmCreateFallsThroughToNewAlarm(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listInstanceAlarms": {
			"data": map[string]any{
				"instanceAlarms": map[string]any{
					"cursor": map[string]any{"next": ""},
					"items":  []map[string]any{},
				},
			},
		},
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"successful": true,
					"result": map[string]any{
						"id":              "fresh-uuid",
						"displayName":     "New Alarm",
						"cloudResourceId": "arn:::brand-new",
					},
				},
			},
		},
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "fresh-uuid",
					"displayName":     "New Alarm",
					"cloudResourceId": "arn:::brand-new",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::brand-new",
		"display_name":      "New Alarm",
		"package_id":        "ecomm-prod-db",
	})

	if diags := resourcePackageAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "fresh-uuid" {
		t.Errorf("expected ID from createInstanceAlarm; got %q want fresh-uuid", rd.Id())
	}
	if rec.FindRequest("createInstanceAlarm") == nil {
		t.Error("createInstanceAlarm must fire when the self-heal lookup finds nothing")
	}
	if rd.Get("last_updated").(string) == "" {
		t.Error("last_updated should be populated after a successful create")
	}
}

// A transport-level failure on the self-heal lookup must NOT be hidden — it
// surfaces verbatim so the user can debug the underlying problem instead of
// being told to migrate.
func TestResourcePackageAlarmCreatePropagatesAPIError(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"listInstanceAlarms": {
			"errors": []map[string]any{
				{"message": "internal server error"},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::target",
		"display_name":      "RDS High CPU",
		"package_id":        "ecomm-prod-db",
	})

	diags := resourcePackageAlarmCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error from the API failure, got none")
	}
	if !strings.Contains(diags[0].Summary, "internal server error") {
		t.Errorf("expected upstream error to be surfaced; got %q", diags[0].Summary)
	}
}

// Without a package_id and without MASSDRIVER_PACKAGE_NAME, the lookup has
// nothing to filter on. Surface a clear error rather than fanning out a
// query that lists every alarm in the org.
func TestResourcePackageAlarmCreateRequiresPackageID(t *testing.T) {
	t.Setenv("MASSDRIVER_PACKAGE_NAME", "")
	pc, rec := newMockProvider(map[string]map[string]any{})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::target",
		"display_name":      "x",
		// no package_id
	})

	diags := resourcePackageAlarmCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error about missing package_id")
	}
	if !strings.Contains(diags[0].Summary, "package_id") && !strings.Contains(diags[0].Summary, "MASSDRIVER_PACKAGE_NAME") {
		t.Errorf("error %q should mention the missing package identifier", diags[0].Summary)
	}
	if len(rec.Requests) != 0 {
		t.Errorf("no API call should fire when package_id is unresolved; got %d", len(rec.Requests))
	}
}

// HCL `package_id` (or MASSDRIVER_PACKAGE_NAME) carries the deployment-suffixed
// package name like `bundtst-plygrnd-awsaurorapos-rbpt`; the instance lookup
// needs the short form (`bundtst-plygrnd-awsaurorapos`). Confirm the strip.
func TestResourcePackageAlarmCreateStripsDeploymentSuffix(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listInstanceAlarms": {
			"data": map[string]any{
				"instanceAlarms": map[string]any{
					"cursor": map[string]any{"next": ""},
					"items":  []map[string]any{},
				},
			},
		},
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"successful": true,
					"result": map[string]any{
						"id":              "fresh-uuid",
						"displayName":     "x",
						"cloudResourceId": "arn:::x",
					},
				},
			},
		},
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "fresh-uuid",
					"displayName":     "x",
					"cloudResourceId": "arn:::x",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "arn:::x",
		"display_name":      "x",
		"package_id":        "bundtst-plygrnd-awsaurorapos-rbpt",
	})

	if diags := resourcePackageAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	vars := gqlmock.Variables(rec.FindRequest("listInstanceAlarms"))
	filter, _ := vars["filter"].(map[string]any)
	instanceID, _ := filter["instanceId"].(map[string]any)
	if got := instanceID["eq"]; got != "bundtst-plygrnd-awsaurorapos" {
		t.Errorf("self-heal filtered on instanceId %v, want bundtst-plygrnd-awsaurorapos (stripped)", got)
	}
	createVars := gqlmock.Variables(rec.FindRequest("createInstanceAlarm"))
	if got := createVars["instanceId"]; got != "bundtst-plygrnd-awsaurorapos" {
		t.Errorf("createInstanceAlarm instanceId %v, want stripped short name", got)
	}
}

// Verifies the field translation between the package_alarm HCL schema and
// CreateInstanceAlarmInput: period_minutes × 60 → period (seconds), metric
// dimensions map → list, etc.
func TestResourcePackageAlarmCreateMapsFieldsToInput(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"listInstanceAlarms": {
			"data": map[string]any{
				"instanceAlarms": map[string]any{
					"cursor": map[string]any{"next": ""},
					"items":  []map[string]any{},
				},
			},
		},
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"successful": true,
					"result": map[string]any{
						"id":              "fresh-uuid",
						"displayName":     "RDS High CPU",
						"cloudResourceId": "arn:::target",
					},
				},
			},
		},
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "fresh-uuid",
					"displayName":     "RDS High CPU",
					"cloudResourceId": "arn:::target",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id":   "arn:::target",
		"display_name":        "RDS High CPU",
		"package_id":          "ecomm-prod-db",
		"threshold":           80.0,
		"period_minutes":      5,
		"comparison_operator": "GreaterThanThreshold",
		"metric": []any{map[string]any{
			"name":      "CPUUtilization",
			"namespace": "AWS/RDS",
			"statistic": "Average",
			"dimensions": map[string]any{
				"DBInstanceIdentifier": "prod-db",
			},
		}},
	})

	if diags := resourcePackageAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	vars := gqlmock.Variables(rec.FindRequest("createInstanceAlarm"))
	input, _ := vars["input"].(map[string]any)

	if input["cloudResourceId"] != "arn:::target" {
		t.Errorf("got cloudResourceId %v", input["cloudResourceId"])
	}
	if input["displayName"] != "RDS High CPU" {
		t.Errorf("got displayName %v", input["displayName"])
	}
	if input["threshold"] != 80.0 {
		t.Errorf("got threshold %v", input["threshold"])
	}
	if got := input["period"]; got != 300.0 { // JSON-decoded ints come back as float64
		t.Errorf("got period %v, want 300 (5 minutes × 60)", got)
	}
	if input["comparisonOperator"] != "GreaterThanThreshold" {
		t.Errorf("got comparisonOperator %v", input["comparisonOperator"])
	}

	metric, _ := input["metric"].(map[string]any)
	if metric["name"] != "CPUUtilization" || metric["namespace"] != "AWS/RDS" {
		t.Errorf("got metric %+v", metric)
	}
	dims, _ := metric["dimensions"].([]any)
	if len(dims) != 1 {
		t.Fatalf("got %d dimensions, want 1", len(dims))
	}
	d0, _ := dims[0].(map[string]any)
	if d0["name"] != "DBInstanceIdentifier" || d0["value"] != "prod-db" {
		t.Errorf("got dimension %+v", d0)
	}
}

func TestResourcePackageAlarmUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateInstanceAlarm": {
			"data": map[string]any{
				"updateInstanceAlarm": map[string]any{
					"successful": true,
					"result": map[string]any{
						"id":              "alarm-uuid",
						"displayName":     "Updated Name",
						"cloudResourceId": "arn:::target",
						"threshold":       95.0,
						"period":          600,
					},
				},
			},
		},
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "alarm-uuid",
					"displayName":     "Updated Name",
					"cloudResourceId": "arn:::target",
					"threshold":       95.0,
					"period":          600,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id":   "arn:::target",
		"display_name":        "Updated Name",
		"threshold":           95.0,
		"period_minutes":      10,
		"comparison_operator": "GreaterThanThreshold",
	})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	vars := gqlmock.Variables(rec.FindRequest("updateInstanceAlarm"))
	if vars["id"] != "alarm-uuid" {
		t.Errorf("got id %v, want alarm-uuid", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["displayName"] != "Updated Name" {
		t.Errorf("got displayName %v", input["displayName"])
	}
	if input["threshold"] != 95.0 {
		t.Errorf("got threshold %v", input["threshold"])
	}
	if got := input["period"]; got != 600.0 {
		t.Errorf("got period %v, want 600 (10 minutes × 60)", got)
	}
	if rd.Get("last_updated").(string) == "" {
		t.Error("last_updated should be populated after a successful update")
	}
}

// All four CRUD contexts must remain wired so terraform can refresh, plan,
// apply, and destroy.
func TestResourcePackageAlarmAllContextsWired(t *testing.T) {
	r := resourcePackageAlarm()
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
	if !strings.Contains(r.DeprecationMessage, "massdriver_instance_alarm") {
		t.Errorf("DeprecationMessage should point at massdriver_instance_alarm; got %q", r.DeprecationMessage)
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
// plans a recreate via the Create path (which runs the self-heal lookup, then
// either adopts or creates via createInstanceAlarm).
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

// When state matches what the API returns, Read must produce zero drift on
// user-facing fields — otherwise every plan would manufacture a fake Update
// even when the user changed nothing.
func TestResourcePackageAlarmReadHydratesStateCleanly(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":                 "alarm-uuid",
					"displayName":        "RDS High CPU",
					"cloudResourceId":    "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
					"comparisonOperator": "GreaterThanThreshold",
					"threshold":          80.0,
					"period":             300, // 5 minutes
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

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id":   "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		"display_name":        "RDS High CPU",
		"threshold":           80.0,
		"period_minutes":      5,
		"comparison_operator": "GreaterThanThreshold",
		"metric": []any{map[string]any{
			"name":      "CPUUtilization",
			"namespace": "AWS/RDS",
			"statistic": "Average",
			"dimensions": map[string]any{
				"DBInstanceIdentifier": "prod-db",
			},
		}},
	})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	for field, want := range map[string]any{
		"cloud_resource_id":   "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		"display_name":        "RDS High CPU",
		"threshold":           80.0,
		"period_minutes":      5,
		"comparison_operator": "GreaterThanThreshold",
	} {
		if got := rd.Get(field); got != want {
			t.Errorf("Read hydrated %s=%v, want %v (would surface as fake drift)", field, got, want)
		}
	}

	metric := rd.Get("metric").([]any)
	if len(metric) != 1 {
		t.Fatalf("got %d metric blocks, want 1", len(metric))
	}
	m := metric[0].(map[string]any)
	if m["name"] != "CPUUtilization" || m["namespace"] != "AWS/RDS" || m["statistic"] != "Average" {
		t.Errorf("metric block hydrated as %+v, doesn't match HCL", m)
	}
	dims := m["dimensions"].(map[string]any)
	if dims["DBInstanceIdentifier"] != "prod-db" {
		t.Errorf("dimensions hydrated as %v, want DBInstanceIdentifier=prod-db", dims)
	}
}

// Read must NOT touch `last_updated` — refreshing it with `time.Now()` would
// make every plan show a diff for a field the user didn't change.
func TestResourcePackageAlarmReadDoesNotChurnLastUpdated(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "alarm-uuid",
					"displayName":     "x",
					"cloudResourceId": "arn:::x",
				},
			},
		},
	})

	const stable = "Mon, 02 Jan 2006 15:04:05 MST"
	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{})
	rd.SetId("alarm-uuid")
	if err := rd.Set("last_updated", stable); err != nil {
		t.Fatal(err)
	}

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := rd.Get("last_updated").(string); got != stable {
		t.Errorf("Read overwrote last_updated to %q, want %q (must not churn)", got, stable)
	}
}

// When the API returns no metric, Read must explicitly clear the block —
// otherwise stale state hides server-side metric removal forever.
func TestResourcePackageAlarmReadClearsMetricWhenAbsent(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "alarm-uuid",
					"displayName":     "Alertmanager Page",
					"cloudResourceId": "alertmanager:my-alert",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourcePackageAlarm().Schema, map[string]any{
		"cloud_resource_id": "alertmanager:my-alert",
		"display_name":      "Alertmanager Page",
		"metric": []any{map[string]any{
			"name":      "stale",
			"namespace": "stale",
		}},
	})
	rd.SetId("alarm-uuid")

	if diags := resourcePackageAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if metric := rd.Get("metric").([]any); len(metric) != 0 {
		t.Errorf("metric should be cleared when API returns no metric; got %+v", metric)
	}
}

// Legacy timestamp IDs can't be deleted server-side. Delete clears state.
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
