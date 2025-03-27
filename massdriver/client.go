package massdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type SnsInterface interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type HttpInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

type EventPublisher interface {
	Publish(event *Event, diags *diag.Diagnostics) error
}

type SnsPublisher struct {
	SnsClient     SnsInterface
	EventTopicARN string
	DeploymentID  string
}

type HttpPublisher struct {
	Client         HttpInterface
	OrganizationID string
	ApiKey         string
}

type MassdriverClient struct {
	Publisher EventPublisher
}

const EVENT_TYPE_ARTIFACT_CREATED string = "artifact_created"
const EVENT_TYPE_ARTIFACT_UPDATED string = "artifact_updated"
const EVENT_TYPE_ARTIFACT_DELETED string = "artifact_deleted"
const EVENT_TYPE_ALARM_CHANNEL_CREATED string = "package_alarm_created"
const EVENT_TYPE_ALARM_CHANNEL_UPDATED string = "package_alarm_updated"
const EVENT_TYPE_ALARM_CHANNEL_DELETED string = "package_alarm_deleted"

func NewMassdriverClient(providerConfig *schema.ResourceData) (*MassdriverClient, error) {
	deploymentID := providerConfig.Get("deployment_id").(string)
	token := providerConfig.Get("token").(string)
	eventTopicARN := providerConfig.Get("event_topic_arn").(string)
	organizationID := providerConfig.Get("organization_id").(string)
	apiKey := providerConfig.Get("api_key").(string)

	// SNS mode
	if deploymentID != "" && token != "" && eventTopicARN != "" {
		awsEndpoint := os.Getenv("MASSDRIVER_AWS_ENDPOINT")
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if awsEndpoint != "" {
				return aws.Endpoint{
					PartitionID:   "aws",
					SigningRegion: "us-east-1",
					URL:           awsEndpoint,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		})

		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithEndpointResolverWithOptions(customResolver))
		if err != nil {
			return nil, fmt.Errorf("error loading AWS config: %w", err)
		}

		snsClient := sns.NewFromConfig(cfg)
		publisher := &SnsPublisher{
			SnsClient:     snsClient,
			EventTopicARN: eventTopicARN,
			DeploymentID:  deploymentID,
		}

		return &MassdriverClient{Publisher: publisher}, nil
	}

	// HTTP API mode
	if organizationID != "" && apiKey != "" {
		publisher := &HttpPublisher{
			Client:         &http.Client{Timeout: 10 * time.Second},
			OrganizationID: organizationID,
			ApiKey:         apiKey,
		}

		return &MassdriverClient{Publisher: publisher}, nil
	}

	return nil, errors.New("invalid configuration: provide either SNS config or API config")
}

func (h *HttpPublisher) Publish(event *Event, diags *diag.Diagnostics) error {
	eventJson, err := json.Marshal(event)
	if err != nil {
		*diags = diag.FromErr(err)
		return err
	}

	// TODO need to configure endpoint based off of event type
	url := fmt.Sprintf("https://api.massdriver.cloud/orgs/%s/artifact", h.OrganizationID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(eventJson))
	if err != nil {
		*diags = diag.FromErr(err)
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.ApiKey))

	resp, err := h.Client.Do(req)
	if err != nil {
		*diags = diag.FromErr(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(bodyBytes))
		*diags = diag.FromErr(err)
		return err
	}

	return nil
}

func (s *SnsPublisher) Publish(event *Event, diags *diag.Diagnostics) error {
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		*diags = diag.FromErr(err)
		return err
	}

	jsonString := string(jsonBytes)
	deduplicationId := uuid.New().String()

	input := &sns.PublishInput{
		Message:                &jsonString,
		MessageDeduplicationId: &deduplicationId,
		MessageGroupId:         &s.DeploymentID,
		TopicArn:               &s.EventTopicARN,
	}

	if s.EventTopicARN != "" {
		_, err = s.SnsClient.Publish(context.Background(), input)
		*diags = diag.FromErr(err)
		return err
	}

	eventBytes, _ := json.Marshal(*input)
	*diags = append(*diags, diag.Diagnostic{
		Severity: diag.Warning,
		Summary:  "Development Override in effect. Resource will not be updated in Massdriver.",
		Detail:   string(eventBytes),
	})

	return nil
}

type Event struct {
	Metadata EventMetadata `json:"metadata"`
	Payload  interface{}   `json:"payload,omitempty"`
}

type EventMetadata struct {
	Timestamp   string `json:"timestamp"`
	Provisioner string `json:"provisioner"`
	Version     string `json:"version,omitempty"`
	EventType   string `json:"event_type"`
}

type EventPayloadArtifacts struct {
	DeploymentId string                 `json:"deployment_id"`
	Artifact     map[string]interface{} `json:"artifact"`
}

type EventPayloadAlarmChannels struct {
	DeploymentId string               `json:"deployment_id"`
	PackageAlarm PackageAlarmMetadata `json:"package_alarm"`
}

var EventTimeString = time.Now().String

func NewEvent(eventType string) *Event {
	event := new(Event)
	event.Metadata.EventType = eventType
	event.Metadata.Timestamp = EventTimeString()
	event.Metadata.Provisioner = os.Getenv("MASSDRIVER_PROVISIONER")
	return event
}
