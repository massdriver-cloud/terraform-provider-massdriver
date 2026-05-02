package api

import (
	"context"
	"fmt"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
)

// InstanceAlarm is a cloud metric alarm attached to an instance. Massdriver
// receives state updates via webhooks from AWS CloudWatch, Azure Monitor,
// GCP Cloud Monitoring, and Prometheus Alertmanager.
//
// Replaces the v0 concept of "package alarm".
type InstanceAlarm struct {
	ID                 string       `json:"id" mapstructure:"id"`
	DisplayName        string       `json:"displayName" mapstructure:"displayName"`
	CloudResourceID    string       `json:"cloudResourceId" mapstructure:"cloudResourceId"`
	ComparisonOperator string       `json:"comparisonOperator,omitempty" mapstructure:"comparisonOperator"`
	Threshold          float64      `json:"threshold,omitempty" mapstructure:"threshold"`
	Period             int          `json:"period,omitempty" mapstructure:"period"`
	Metric             *AlarmMetric `json:"metric,omitempty" mapstructure:"metric,omitempty"`
}

// AlarmMetric describes the cloud metric an alarm evaluates. Field availability
// depends on the cloud provider; expect partial population.
type AlarmMetric struct {
	Namespace  string                 `json:"namespace,omitempty" mapstructure:"namespace"`
	Name       string                 `json:"name,omitempty" mapstructure:"name"`
	Statistic  string                 `json:"statistic,omitempty" mapstructure:"statistic"`
	Region     string                 `json:"region,omitempty" mapstructure:"region"`
	Dimensions []AlarmMetricDimension `json:"dimensions,omitempty" mapstructure:"dimensions"`
}

// AlarmMetricDimension is a key/value pair identifying the cloud resource a metric applies to.
type AlarmMetricDimension struct {
	Name  string `json:"name" mapstructure:"name"`
	Value string `json:"value" mapstructure:"value"`
}

// GetInstanceAlarm retrieves an alarm by ID.
func GetInstanceAlarm(ctx context.Context, mdClient *client.Client, id string) (*InstanceAlarm, error) {
	response, err := getInstanceAlarm(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance alarm %s: %w", id, err)
	}
	return toInstanceAlarm(response.InstanceAlarm)
}

// CreateInstanceAlarm registers an alarm against an existing instance.
func CreateInstanceAlarm(ctx context.Context, mdClient *client.Client, instanceID string, input CreateInstanceAlarmInput) (*InstanceAlarm, error) {
	response, err := createInstanceAlarm(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, instanceID, input)
	if err != nil {
		return nil, err
	}
	if !response.CreateInstanceAlarm.Successful {
		messages := make([]string, 0, len(response.CreateInstanceAlarm.Messages))
		for _, m := range response.CreateInstanceAlarm.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to create instance alarm", messages)
	}
	return toInstanceAlarm(response.CreateInstanceAlarm.Result)
}

// UpdateInstanceAlarm updates an existing alarm. Pass only the fields you want to change.
func UpdateInstanceAlarm(ctx context.Context, mdClient *client.Client, id string, input UpdateInstanceAlarmInput) (*InstanceAlarm, error) {
	response, err := updateInstanceAlarm(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id, input)
	if err != nil {
		return nil, err
	}
	if !response.UpdateInstanceAlarm.Successful {
		messages := make([]string, 0, len(response.UpdateInstanceAlarm.Messages))
		for _, m := range response.UpdateInstanceAlarm.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to update instance alarm", messages)
	}
	return toInstanceAlarm(response.UpdateInstanceAlarm.Result)
}

// DeleteInstanceAlarm removes an alarm registration. The underlying cloud provider alarm is unaffected.
func DeleteInstanceAlarm(ctx context.Context, mdClient *client.Client, id string) (*InstanceAlarm, error) {
	response, err := deleteInstanceAlarm(ctx, mdClient.GQLv1, mdClient.Config.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !response.DeleteInstanceAlarm.Successful {
		messages := make([]string, 0, len(response.DeleteInstanceAlarm.Messages))
		for _, m := range response.DeleteInstanceAlarm.Messages {
			messages = append(messages, m.Message)
		}
		return nil, mutationFailure("unable to delete instance alarm", messages)
	}
	return toInstanceAlarm(response.DeleteInstanceAlarm.Result)
}

func toInstanceAlarm(v any) (*InstanceAlarm, error) {
	a := InstanceAlarm{}
	if err := decode(v, &a); err != nil {
		return nil, fmt.Errorf("failed to decode instance alarm: %w", err)
	}
	return &a, nil
}
