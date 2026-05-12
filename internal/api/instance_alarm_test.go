package api_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	api "terraform-provider-massdriver/internal/api"
	"terraform-provider-massdriver/internal/gqlmock"
)

func TestGetInstanceAlarm(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"instanceAlarm": map[string]any{
				"id":                 "alarm-uuid1",
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
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.GetInstanceAlarm(t.Context(), &mdClient, "alarm-uuid1")
	if err != nil {
		t.Fatal(err)
	}

	if alarm.ID != "alarm-uuid1" {
		t.Errorf("got ID %s, wanted alarm-uuid1", alarm.ID)
	}
	if alarm.DisplayName != "RDS High CPU" {
		t.Errorf("got displayName %q, wanted RDS High CPU", alarm.DisplayName)
	}
	if alarm.CloudResourceID != "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu" {
		t.Errorf("got cloudResourceId %q", alarm.CloudResourceID)
	}
	if alarm.ComparisonOperator != "GREATER_THAN" {
		t.Errorf("got comparisonOperator %q, wanted GREATER_THAN", alarm.ComparisonOperator)
	}
	if alarm.Threshold != 80.0 {
		t.Errorf("got threshold %v, wanted 80.0", alarm.Threshold)
	}
	if alarm.Period != 300 {
		t.Errorf("got period %v, wanted 300", alarm.Period)
	}
	if alarm.Metric == nil {
		t.Fatal("expected metric, got nil")
	}
	if alarm.Metric.Name != "CPUUtilization" || alarm.Metric.Namespace != "AWS/RDS" {
		t.Errorf("got metric %+v, wanted CPUUtilization on AWS/RDS", alarm.Metric)
	}
	if len(alarm.Metric.Dimensions) != 1 || alarm.Metric.Dimensions[0].Name != "DBInstanceIdentifier" {
		t.Errorf("expected one DBInstanceIdentifier dimension, got %+v", alarm.Metric.Dimensions)
	}
}

// Many alarms (Alertmanager, some GCP conditions) lack a structured metric.
// Metric is pointer-typed so callers can detect "not populated" rather than
// "populated with zero values" — load-bearing for the package_alarm Read,
// which omits the metric block entirely when the API returns null.
func TestGetInstanceAlarm_HandlesMissingMetric(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"instanceAlarm": map[string]any{
				"id":              "alarm-no-metric",
				"displayName":     "Alertmanager Page",
				"cloudResourceId": "alertmanager:my-alert",
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.GetInstanceAlarm(t.Context(), &mdClient, "alarm-no-metric")
	if err != nil {
		t.Fatal(err)
	}
	if alarm.Metric != nil {
		t.Errorf("metric should be nil when absent, got %+v", alarm.Metric)
	}
}

func TestFindInstanceAlarmByCloudResourceID(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"instanceAlarms": map[string]any{
				"cursor": map[string]any{"next": ""},
				"items": []map[string]any{
					{
						"id":              "wrong-uuid",
						"displayName":     "Other Alarm",
						"cloudResourceId": "arn:::other",
					},
					{
						"id":              "right-uuid",
						"displayName":     "Target Alarm",
						"cloudResourceId": "arn:::target",
					},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.FindInstanceAlarmByCloudResourceID(t.Context(), &mdClient, "ecomm-prod-db", "arn:::target")
	if err != nil {
		t.Fatal(err)
	}
	if alarm == nil {
		t.Fatal("expected to find target alarm, got nil")
	}
	if alarm.ID != "right-uuid" {
		t.Errorf("got ID %q, wanted right-uuid", alarm.ID)
	}
}

// Pagination has to actually work — a hard-coded single-page lookup would
// silently fail to find alarms past the page boundary, surfacing as a
// confusing "not found" right when the recovery path is supposed to fire.
func TestFindInstanceAlarmByCloudResourceID_FollowsCursor(t *testing.T) {
	page := 0
	gqlClient := gqlClientFunc(func(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
		page++
		var body string
		switch page {
		case 1:
			body = `{"data":{"instanceAlarms":{"cursor":{"next":"page2"},"items":[{"id":"page1-uuid","cloudResourceId":"arn:::not-target"}]}}}`
		case 2:
			body = `{"data":{"instanceAlarms":{"cursor":{"next":""},"items":[{"id":"page2-uuid","cloudResourceId":"arn:::target"}]}}}`
		default:
			t.Fatalf("unexpected page request %d", page)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &envelope); err != nil {
			return err
		}
		return json.Unmarshal(envelope.Data, resp.Data)
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.FindInstanceAlarmByCloudResourceID(t.Context(), &mdClient, "ecomm-prod-db", "arn:::target")
	if err != nil {
		t.Fatal(err)
	}
	if alarm == nil || alarm.ID != "page2-uuid" {
		t.Errorf("expected to find page2-uuid, got %+v", alarm)
	}
	if page != 2 {
		t.Errorf("expected 2 page requests, got %d", page)
	}
}

// gqlClientFunc adapts a function into a graphql.Client. Used by the
// pagination test to return different canned responses per invocation —
// the shared gqlmock.Recorder helpers only support a single response per
// operation name.
type gqlClientFunc func(context.Context, *graphql.Request, *graphql.Response) error

func (f gqlClientFunc) MakeRequest(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
	return f(ctx, req, resp)
}

// (nil, nil) is the contract for "no match found" — distinct from a
// transport-level error. The package_alarm Create relies on this to
// distinguish "user is genuinely creating something new" from "API is down".
func TestFindInstanceAlarmByCloudResourceID_ReturnsNilWhenMissing(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"instanceAlarms": map[string]any{
				"cursor": map[string]any{"next": ""},
				"items": []map[string]any{
					{"id": "x", "cloudResourceId": "arn:::different"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.FindInstanceAlarmByCloudResourceID(t.Context(), &mdClient, "ecomm-prod-db", "arn:::nope")
	if err != nil {
		t.Fatal(err)
	}
	if alarm != nil {
		t.Errorf("expected nil for missing alarm, got %+v", alarm)
	}
}

func TestCreateInstanceAlarm(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createInstanceAlarm": map[string]any{
				"result": map[string]any{
					"id":              "alarm-new",
					"displayName":     "RDS High CPU",
					"cloudResourceId": "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	threshold := 80.0
	period := 300
	alarm, err := api.CreateInstanceAlarm(t.Context(), &mdClient, "ecomm-prod-db", api.CreateInstanceAlarmInput{
		CloudResourceId:    "arn:aws:cloudwatch:us-east-1:111:alarm/rds-cpu",
		DisplayName:        "RDS High CPU",
		ComparisonOperator: "GREATER_THAN",
		Threshold:          &threshold,
		Period:             &period,
	})
	if err != nil {
		t.Fatal(err)
	}
	if alarm.ID != "alarm-new" {
		t.Errorf("got ID %s, wanted alarm-new", alarm.ID)
	}
}

// A failed mutation (`successful: false`) must surface as a Go-level error
// carrying the server's validation messages, not be silently swallowed.
func TestCreateInstanceAlarmFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"createInstanceAlarm": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "validation", "field": "cloudResourceId", "message": "must be unique within instance"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.CreateInstanceAlarm(t.Context(), &mdClient, "ecomm-prod-db", api.CreateInstanceAlarmInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateInstanceAlarm(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"updateInstanceAlarm": map[string]any{
				"result": map[string]any{
					"id":          "alarm-1",
					"displayName": "Renamed",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.UpdateInstanceAlarm(t.Context(), &mdClient, "alarm-1", api.UpdateInstanceAlarmInput{
		DisplayName: "Renamed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if alarm.DisplayName != "Renamed" {
		t.Errorf("got displayName %q, wanted Renamed", alarm.DisplayName)
	}
}

func TestDeleteInstanceAlarm(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteInstanceAlarm": map[string]any{
				"result": map[string]any{
					"id":          "alarm-1",
					"displayName": "RDS High CPU",
				},
				"successful": true,
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	alarm, err := api.DeleteInstanceAlarm(t.Context(), &mdClient, "alarm-1")
	if err != nil {
		t.Fatal(err)
	}
	if alarm.ID != "alarm-1" {
		t.Errorf("got ID %s, wanted alarm-1", alarm.ID)
	}
}

func TestDeleteInstanceAlarmFailure(t *testing.T) {
	gqlClient := gqlmock.NewClientWithSingleJSONResponse(map[string]any{
		"data": map[string]any{
			"deleteInstanceAlarm": map[string]any{
				"result":     nil,
				"successful": false,
				"messages": []map[string]any{
					{"code": "not_found", "field": "id", "message": "alarm not found"},
				},
			},
		},
	})
	mdClient := client.Client{GQLv2: gqlClient}

	_, err := api.DeleteInstanceAlarm(t.Context(), &mdClient, "alarm-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
