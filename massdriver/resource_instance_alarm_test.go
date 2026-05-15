package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/instances"
)

// fakeInstanceAlarms records every call for assertion and returns whatever
// canned response the test wires in. It satisfies instanceAlarmsAPI.
type fakeInstanceAlarms struct {
	// Canned responses.
	getResp, createResp, updateResp, deleteResp *instances.Alarm
	getErr, createErr, updateErr, deleteErr     error

	// Captured arguments.
	getID            string
	createInstanceID string
	createInput      instances.CreateAlarmInput
	updateID         string
	updateInput      instances.UpdateAlarmInput
	deleteID         string

	// Call counts (useful for "must not be called" assertions).
	getCalls, createCalls, updateCalls, deleteCalls int
}

func (f *fakeInstanceAlarms) GetAlarm(_ context.Context, id string) (*instances.Alarm, error) {
	f.getID = id
	f.getCalls++
	return f.getResp, f.getErr
}

func (f *fakeInstanceAlarms) CreateAlarm(_ context.Context, instanceID string, input instances.CreateAlarmInput) (*instances.Alarm, error) {
	f.createInstanceID = instanceID
	f.createInput = input
	f.createCalls++
	return f.createResp, f.createErr
}

func (f *fakeInstanceAlarms) UpdateAlarm(_ context.Context, id string, input instances.UpdateAlarmInput) (*instances.Alarm, error) {
	f.updateID = id
	f.updateInput = input
	f.updateCalls++
	return f.updateResp, f.updateErr
}

func (f *fakeInstanceAlarms) DeleteAlarm(_ context.Context, id string) (*instances.Alarm, error) {
	f.deleteID = id
	f.deleteCalls++
	return f.deleteResp, f.deleteErr
}

// fullAlarm is the canonical canned response used by Create→Read and Update→Read tests.
func fullAlarm() *instances.Alarm {
	return &instances.Alarm{
		ID:                 "alarm-1",
		DisplayName:        "RDS High CPU",
		CloudResourceID:    "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		ComparisonOperator: "GREATER_THAN",
		Threshold:          80.0,
		Period:             300,
		Metric: &instances.AlarmMetric{
			Namespace: "AWS/RDS",
			Name:      "CPUUtilization",
			Statistic: "Average",
			Region:    "us-east-1",
			Dimensions: []instances.AlarmMetricDimension{
				{Name: "DBInstanceIdentifier", Value: "prod-db"},
			},
		},
	}
}

func TestResourceInstanceAlarmCreate(t *testing.T) {
	fake := &fakeInstanceAlarms{
		createResp: fullAlarm(),
		getResp:    fullAlarm(),
	}
	pc := &ProviderClient{InstanceAlarms: fake}

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
	if fake.createCalls != 1 {
		t.Fatalf("CreateAlarm called %d times, want 1", fake.createCalls)
	}
	if fake.createInstanceID != "ecomm-prod-db" {
		t.Errorf("got instanceID %q, want ecomm-prod-db", fake.createInstanceID)
	}

	in := fake.createInput
	if in.CloudResourceID != "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu" {
		t.Errorf("got cloudResourceID %q", in.CloudResourceID)
	}
	if in.DisplayName != "RDS High CPU" {
		t.Errorf("got displayName %q", in.DisplayName)
	}
	if in.ComparisonOperator != "GREATER_THAN" {
		t.Errorf("got comparisonOperator %q", in.ComparisonOperator)
	}
	if in.Threshold == nil || *in.Threshold != 80.0 {
		t.Errorf("got threshold %v, want *80.0", in.Threshold)
	}
	if in.Period == nil || *in.Period != 300 {
		t.Errorf("got period %v, want *300", in.Period)
	}
	if in.Metric == nil {
		t.Fatal("metric should be populated")
	}
	if in.Metric.Namespace != "AWS/RDS" || in.Metric.Name != "CPUUtilization" {
		t.Errorf("got metric %+v", in.Metric)
	}
	if len(in.Metric.Dimensions) != 1 || in.Metric.Dimensions[0].Name != "DBInstanceIdentifier" {
		t.Errorf("got dimensions %+v", in.Metric.Dimensions)
	}
}

// Alertmanager and some GCP alarms have no comparison_operator/threshold/period/metric.
// We must leave those nil/empty on the SDK input rather than send zero values
// that the backend would interpret as "the user set 0".
func TestResourceInstanceAlarmCreateOmitsUnsetOptionalFields(t *testing.T) {
	resp := &instances.Alarm{
		ID:              "alarm-2",
		DisplayName:     "Alertmanager Page",
		CloudResourceID: "alertmanager:my-alert",
	}
	fake := &fakeInstanceAlarms{createResp: resp, getResp: resp}
	pc := &ProviderClient{InstanceAlarms: fake}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"instance_id":       "ecomm-prod-db",
		"display_name":      "Alertmanager Page",
		"cloud_resource_id": "alertmanager:my-alert",
	})

	if diags := resourceInstanceAlarmCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	in := fake.createInput
	if in.ComparisonOperator != "" {
		t.Errorf("comparisonOperator should be empty, got %q", in.ComparisonOperator)
	}
	if in.Threshold != nil {
		t.Errorf("threshold should be nil, got %v", *in.Threshold)
	}
	if in.Period != nil {
		t.Errorf("period should be nil, got %v", *in.Period)
	}
	if in.Metric != nil {
		t.Errorf("metric should be nil, got %+v", in.Metric)
	}
}

func TestResourceInstanceAlarmCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeInstanceAlarms{
		createErr: fmt.Errorf("create instance alarm: cloudResourceId must be unique within instance"),
	}
	pc := &ProviderClient{InstanceAlarms: fake}

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
	if !strings.Contains(diags[0].Summary, "must be unique") {
		t.Errorf("upstream error %q should be surfaced verbatim", diags[0].Summary)
	}
}

// Without HCL config or env vars, Create must surface a clear error rather
// than firing an API call with an empty instance ID.
func TestResourceInstanceAlarmCreateRequiresInstanceID(t *testing.T) {
	t.Setenv("MASSDRIVER_INSTANCE_ID", "")
	t.Setenv("MASSDRIVER_PACKAGE_NAME", "")
	fake := &fakeInstanceAlarms{}
	pc := &ProviderClient{InstanceAlarms: fake}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		"display_name":      "x",
		"cloud_resource_id": "arn:::x",
		// no instance_id
	})

	diags := resourceInstanceAlarmCreate(t.Context(), rd, pc)
	if !diags.HasError() {
		t.Fatal("expected error about missing instance_id")
	}
	if !strings.Contains(diags[0].Summary, "instance_id") {
		t.Errorf("error %q should mention instance_id", diags[0].Summary)
	}
	if fake.createCalls != 0 {
		t.Errorf("CreateAlarm should not fire when instance_id is unresolved; called %d times", fake.createCalls)
	}
}

func TestResourceInstanceAlarmRead(t *testing.T) {
	pc := &ProviderClient{InstanceAlarms: &fakeInstanceAlarms{getResp: fullAlarm()}}

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

// When the API returns no metric, Read must explicitly clear the metric block
// — otherwise stale state masks a server-side metric removal forever.
func TestResourceInstanceAlarmReadClearsMetricWhenAbsent(t *testing.T) {
	pc := &ProviderClient{InstanceAlarms: &fakeInstanceAlarms{
		getResp: &instances.Alarm{
			ID:              "alarm-no-metric",
			DisplayName:     "Alertmanager Page",
			CloudResourceID: "alertmanager:my-alert",
			Metric:          nil,
		},
	}}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{
		// State already has a metric; API now returns none. Read should clear.
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

// A `gql.ErrNotFound` from Get clears state so terraform plans a recreate.
// Any other error propagates.
func TestResourceInstanceAlarmReadClearsOnNotFound(t *testing.T) {
	pc := &ProviderClient{InstanceAlarms: &fakeInstanceAlarms{
		getErr: fmt.Errorf("get instance alarm gone: %w", gql.ErrNotFound),
	}}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{})
	rd.SetId("gone")

	if diags := resourceInstanceAlarmRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found should clear state silently; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared on not-found; got %q", rd.Id())
	}
}

func TestResourceInstanceAlarmUpdate(t *testing.T) {
	updated := fullAlarm()
	updated.DisplayName = "Renamed"
	fake := &fakeInstanceAlarms{updateResp: updated, getResp: updated}
	pc := &ProviderClient{InstanceAlarms: fake}

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

	if fake.updateID != "alarm-1" {
		t.Errorf("got updateID %q, want alarm-1", fake.updateID)
	}
	in := fake.updateInput
	if in.DisplayName != "Renamed" {
		t.Errorf("got displayName %q, want Renamed", in.DisplayName)
	}
	if in.Threshold == nil || *in.Threshold != 95.0 {
		t.Errorf("got threshold %v, want *95.0", in.Threshold)
	}
}

func TestResourceInstanceAlarmDelete(t *testing.T) {
	fake := &fakeInstanceAlarms{deleteResp: &instances.Alarm{ID: "alarm-1"}}
	pc := &ProviderClient{InstanceAlarms: fake}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{})
	rd.SetId("alarm-1")

	if diags := resourceInstanceAlarmDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("resource ID should be cleared, got %q", rd.Id())
	}
	if fake.deleteID != "alarm-1" {
		t.Errorf("got deleteID %q, want alarm-1", fake.deleteID)
	}
}

// Delete returning not-found is treated as success — the record is gone, which
// is what destroy wanted anyway.
func TestResourceInstanceAlarmDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	fake := &fakeInstanceAlarms{deleteErr: fmt.Errorf("delete instance alarm: %w", gql.ErrNotFound)}
	pc := &ProviderClient{InstanceAlarms: fake}

	rd := schema.TestResourceDataRaw(t, resourceInstanceAlarm().Schema, map[string]any{})
	rd.SetId("already-gone")

	if diags := resourceInstanceAlarmDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("not-found on delete should not error; got %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("ID should be cleared, got %q", rd.Id())
	}
}

// The DefaultFunc resolves instance_id from env: MASSDRIVER_INSTANCE_ID wins;
// MASSDRIVER_PACKAGE_NAME is the fallback and gets the trailing deployment
// suffix stripped. HCL config trumps both via standard SDK precedence.
func TestInstanceIDFromEnv(t *testing.T) {
	cases := []struct {
		name        string
		instanceEnv string
		packageEnv  string
		want        any
	}{
		{name: "both_unset", want: nil},
		{name: "instance_id_explicit", instanceEnv: "explicit-id", want: "explicit-id"},
		{name: "package_name_stripped", packageEnv: "bundtst-plygrnd-awsaurorapos-rbpt", want: "bundtst-plygrnd-awsaurorapos"},
		{name: "instance_id_wins_over_package_name", instanceEnv: "explicit-id", packageEnv: "should-be-ignored-x", want: "explicit-id"},
		{name: "package_name_without_hyphen_unchanged", packageEnv: "singletoken", want: "singletoken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MASSDRIVER_INSTANCE_ID", tc.instanceEnv)
			t.Setenv("MASSDRIVER_PACKAGE_NAME", tc.packageEnv)
			got, err := instanceIDFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResourceInstanceAlarmSchema(t *testing.T) {
	r := resourceInstanceAlarm()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	// instance_id is Optional (resolved by DefaultFunc from env in deployments)
	// but ForceNew — moving an alarm between instances is destroy+recreate.
	if iid := r.Schema["instance_id"]; iid == nil || iid.Required || !iid.Optional || !iid.ForceNew || iid.DefaultFunc == nil {
		t.Error("instance_id should be Optional+ForceNew with a DefaultFunc")
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
	if metric := r.Schema["metric"]; metric.MaxItems != 1 || metric.Type != schema.TypeList {
		t.Errorf("metric should be TypeList with MaxItems=1, got Type=%v MaxItems=%d", metric.Type, metric.MaxItems)
	}
}

// Compile-time assertion: the SDK's *instances.Service must satisfy
// instanceAlarmsAPI. If the SDK changes a signature, the build breaks here
// rather than failing at NewProviderClient assignment.
var _ instanceAlarmsAPI = (*instances.Service)(nil)
