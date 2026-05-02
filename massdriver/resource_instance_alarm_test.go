package massdriver

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/gqlmock"
)

// alarmReadResponse is a canned getInstanceAlarm response used after Create/Update.
func alarmReadResponse(extra map[string]any) map[string]any {
	base := map[string]any{
		"id":                 "alarm-1",
		"displayName":        "RDS High CPU",
		"cloudResourceId":    "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		"comparisonOperator": "GREATER_THAN",
		"threshold":          80.0,
		"period":             300,
		"metric": map[string]any{
			"namespace": "AWS/RDS",
			"name":      "CPUUtilization",
			"statistic": "Average",
			"region":    "us-east-1",
			"dimensions": []map[string]any{
				{"name": "DBInstanceIdentifier", "value": "prod-db"},
			},
		},
	}
	for k, v := range extra {
		base[k] = v
	}
	return map[string]any{
		"data": map[string]any{"instanceAlarm": base},
	}
}

func TestResourceInstanceAlarmCreate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"result": map[string]any{
						"id":              "alarm-1",
						"displayName":     "RDS High CPU",
						"cloudResourceId": "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
					},
					"successful": true,
				},
			},
		},
		"getInstanceAlarm": alarmReadResponse(nil),
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"instance_id":         "ecomm-prod-db",
		"display_name":        "RDS High CPU",
		"cloud_resource_id":   "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		"comparison_operator": "GREATER_THAN",
		"threshold":           80.0,
		"period":              300,
		"metric": []any{
			map[string]any{
				"namespace": "AWS/RDS",
				"name":      "CPUUtilization",
				"statistic": "Average",
				"region":    "us-east-1",
				"dimensions": map[string]any{
					"DBInstanceIdentifier": "prod-db",
				},
			},
		},
	})

	if diags := resourceInstanceAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "alarm-1" {
		t.Errorf("got id %q, want alarm-1", rd.Id())
	}

	createReq := rec.FindRequest("createInstanceAlarm")
	if createReq == nil {
		t.Fatal("createInstanceAlarm was not called")
	}
	vars := gqlmock.Variables(createReq)
	if vars["instanceId"] != "ecomm-prod-db" {
		t.Errorf("got instanceId %v, want ecomm-prod-db", vars["instanceId"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["cloudResourceId"] != "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu" {
		t.Errorf("got cloudResourceId %v", input["cloudResourceId"])
	}
	if input["displayName"] != "RDS High CPU" {
		t.Errorf("got displayName %v", input["displayName"])
	}
	if input["comparisonOperator"] != "GREATER_THAN" {
		t.Errorf("got comparisonOperator %v", input["comparisonOperator"])
	}
	if input["threshold"] != 80.0 {
		t.Errorf("got threshold %v, want 80.0", input["threshold"])
	}
	if input["period"] != float64(300) { // JSON numbers come back as float64
		t.Errorf("got period %v, want 300", input["period"])
	}
	metric, ok := input["metric"].(map[string]any)
	if !ok {
		t.Fatalf("expected metric block, got %T (%v)", input["metric"], input["metric"])
	}
	if metric["namespace"] != "AWS/RDS" || metric["name"] != "CPUUtilization" {
		t.Errorf("got metric %+v", metric)
	}
	dims, _ := metric["dimensions"].([]any)
	if len(dims) != 1 {
		t.Fatalf("got %d dimensions, want 1", len(dims))
	}
	dim := dims[0].(map[string]any)
	if dim["name"] != "DBInstanceIdentifier" || dim["value"] != "prod-db" {
		t.Errorf("got dimension %+v, want DBInstanceIdentifier=prod-db", dim)
	}
}

// Alertmanager and some GCP alarms have no comparison_operator/threshold/period/metric.
// We must omit them from the API request rather than send zero values that would cause
// the backend to reject or store nonsense values.
func TestResourceInstanceAlarmCreateOmitsUnsetOptionalFields(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"result": map[string]any{
						"id":              "alarm-2",
						"displayName":     "Alertmanager Page",
						"cloudResourceId": "alertmanager:my-alert",
					},
					"successful": true,
				},
			},
		},
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "alarm-2",
					"displayName":     "Alertmanager Page",
					"cloudResourceId": "alertmanager:my-alert",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"instance_id":       "ecomm-prod-db",
		"display_name":      "Alertmanager Page",
		"cloud_resource_id": "alertmanager:my-alert",
	})

	if diags := resourceInstanceAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	input, _ := gqlmock.Variables(rec.FindRequest("createInstanceAlarm"))["input"].(map[string]any)
	for _, omitted := range []string{"comparisonOperator", "threshold", "period", "metric"} {
		if _, present := input[omitted]; present {
			t.Errorf("input.%s should be omitted when not set, got %v", omitted, input[omitted])
		}
	}
}

func TestResourceInstanceAlarmCreatePropagatesAPIFailure(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"createInstanceAlarm": {
			"data": map[string]any{
				"createInstanceAlarm": map[string]any{
					"result":     nil,
					"successful": false,
					"messages": []map[string]any{
						{"code": "validation", "field": "cloudResourceId", "message": "must be unique within instance"},
					},
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"instance_id":       "ecomm-prod-db",
		"display_name":      "Dup",
		"cloud_resource_id": "duplicate-arn",
	})

	diags := resourceInstanceAlarmCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error, got none")
	}
	if rd.Id() != "" {
		t.Errorf("ID should not be set on failure, got %q", rd.Id())
	}
}

func TestResourceInstanceAlarmRead(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": alarmReadResponse(nil),
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{})
	rd.SetId("alarm-1")

	if diags := resourceInstanceAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Get("display_name").(string) != "RDS High CPU" {
		t.Errorf("got display_name %q", rd.Get("display_name"))
	}
	if rd.Get("threshold").(float64) != 80.0 {
		t.Errorf("got threshold %v, want 80.0", rd.Get("threshold"))
	}
	if rd.Get("period").(int) != 300 {
		t.Errorf("got period %v, want 300", rd.Get("period"))
	}

	metricList := rd.Get("metric").([]any)
	if len(metricList) != 1 {
		t.Fatalf("got %d metric blocks, want 1", len(metricList))
	}
	metric := metricList[0].(map[string]any)
	if metric["namespace"] != "AWS/RDS" {
		t.Errorf("got metric.namespace %v, want AWS/RDS", metric["namespace"])
	}
	dims := metric["dimensions"].(map[string]any)
	if dims["DBInstanceIdentifier"] != "prod-db" {
		t.Errorf("got dimensions %v", dims)
	}
}

// When the API returns no metric, the resource should clear the metric block in state
// rather than leaving a zero-valued one that would show up as drift on next plan.
func TestResourceInstanceAlarmReadClearsMetricWhenAbsent(t *testing.T) {
	pc, _ := newMockProvider(map[string]map[string]any{
		"getInstanceAlarm": {
			"data": map[string]any{
				"instanceAlarm": map[string]any{
					"id":              "alarm-no-metric",
					"displayName":     "Alertmanager Page",
					"cloudResourceId": "alertmanager:my-alert",
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		// State already has a metric (e.g., user previously had one configured),
		// then the API stopped returning it. Read should clear, not leak the old block.
		"metric": []any{
			map[string]any{"namespace": "stale"},
		},
	})
	rd.SetId("alarm-no-metric")

	if diags := resourceInstanceAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if metric := rd.Get("metric").([]any); len(metric) != 0 {
		t.Errorf("metric should be empty when API returns no metric; got %+v", metric)
	}
}

func TestResourceInstanceAlarmUpdate(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"updateInstanceAlarm": {
			"data": map[string]any{
				"updateInstanceAlarm": map[string]any{
					"result": map[string]any{
						"id":              "alarm-1",
						"displayName":     "Renamed",
						"cloudResourceId": "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
					},
					"successful": true,
				},
			},
		},
		"getInstanceAlarm": alarmReadResponse(map[string]any{"displayName": "Renamed"}),
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"instance_id":       "ecomm-prod-db",
		"display_name":      "Renamed",
		"cloud_resource_id": "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		"threshold":         95.0,
	})
	rd.SetId("alarm-1")

	if diags := resourceInstanceAlarmUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	updateReq := rec.FindRequest("updateInstanceAlarm")
	if updateReq == nil {
		t.Fatal("updateInstanceAlarm was not called")
	}
	vars := gqlmock.Variables(updateReq)
	if vars["id"] != "alarm-1" {
		t.Errorf("got id %v, want alarm-1", vars["id"])
	}
	input, _ := vars["input"].(map[string]any)
	if input["displayName"] != "Renamed" {
		t.Errorf("got displayName %v, want Renamed", input["displayName"])
	}
	if input["threshold"] != 95.0 {
		t.Errorf("got threshold %v, want 95.0", input["threshold"])
	}
}

func TestResourceInstanceAlarmDelete(t *testing.T) {
	pc, rec := newMockProvider(map[string]map[string]any{
		"deleteInstanceAlarm": {
			"data": map[string]any{
				"deleteInstanceAlarm": map[string]any{
					"result":     map[string]any{"id": "alarm-1", "displayName": "RDS High CPU"},
					"successful": true,
				},
			},
		},
	})

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{})
	rd.SetId("alarm-1")

	if diags := resourceInstanceAlarmDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if vars := gqlmock.Variables(rec.FindRequest("deleteInstanceAlarm")); vars["id"] != "alarm-1" {
		t.Errorf("got id %v, want alarm-1", vars["id"])
	}
}

func TestResourceInstanceAlarmSchema(t *testing.T) {
	r := resourceInstanceAlarm()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if iid := r.Schema["instance_id"]; iid == nil || !iid.Required || !iid.ForceNew {
		t.Error("instance_id should be Required+ForceNew")
	}
	for _, field := range []string{"display_name", "cloud_resource_id"} {
		if !r.Schema[field].Required {
			t.Errorf("%s should be Required", field)
		}
	}
	for _, field := range []string{"comparison_operator", "threshold", "period", "metric"} {
		s := r.Schema[field]
		if s == nil {
			t.Fatalf("expected %s in schema", field)
		}
		if !s.Optional {
			t.Errorf("%s should be Optional", field)
		}
	}
	// metric is a single-instance nested block.
	if metric := r.Schema["metric"]; metric.MaxItems != 1 || metric.Type != schema.TypeList {
		t.Errorf("metric should be TypeList with MaxItems=1, got Type=%v MaxItems=%d", metric.Type, metric.MaxItems)
	}
}
