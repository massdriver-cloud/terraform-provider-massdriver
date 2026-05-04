package api_test

import (
	"testing"

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
// Metric is the only field where we preserve null vs. populated (via a pointer)
// because callers need to know whether to emit metric details at all. Threshold
// and period collapse null to 0 — the terraform layer disambiguates with GetOk.
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
