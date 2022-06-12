package massdriver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

type SnsInterface interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type MassdriverClient struct {
	DeploymentID  string
	Token         string
	EventTopicARN string
	SNSClient     SnsInterface
}

const EVENT_TYPE_ARTIFACT_CREATED string = "artifact_created"
const EVENT_TYPE_ARTIFACT_UPDATED string = "artifact_updated"
const EVENT_TYPE_ARTIFACT_DELETED string = "artifact_deleted"
const EVENT_TYPE_ALARM_CHANNEL_CREATED string = "package_alarm_created"
const EVENT_TYPE_ALARM_CHANNEL_UPDATED string = "package_alarm_updated"
const EVENT_TYPE_ALARM_CHANNEL_DELETED string = "package_alarm_deleted"

func NewMassdriverClient(deployment_id, token, event_topic_arn string) (*MassdriverClient, error) {
	c := new(MassdriverClient)

	c.DeploymentID = deployment_id
	c.Token = token
	c.EventTopicARN = event_topic_arn

	awsEndpoint := os.Getenv("MASSDRIVER_AWS_ENDPOINT")
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if awsEndpoint != "" {
			return aws.Endpoint{
				PartitionID:   "aws",
				SigningRegion: "us-east-1",
				URL:           awsEndpoint,
			}, nil
		}
		// returning EndpointNotFoundError will allow the service to fallback to it's default resolution
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	c.SNSClient = sns.NewFromConfig(cfg)

	return c, nil
}

func (c MassdriverClient) PublishEventToSNS(event *Event, diags *diag.Diagnostics) error {
	input, err := c.buildSNSEvent(event)

	if err != nil {
		*diags = diag.FromErr(err)
		return err
	}

	if c.EventTopicARN != "" {
		_, err = c.SNSClient.Publish(context.Background(), &input)
		*diags = diag.FromErr(err)
		return err
	} else {
		eventBytes, _ := json.Marshal(input)
		fmt.Println("test")
		*diags = append(*diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Development Override in effect. Resource will not be updated in Massdriver.",
			Detail:   string(eventBytes),
		})
	}

	return nil
}

func (c MassdriverClient) buildSNSEvent(event *Event) (sns.PublishInput, error) {
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return sns.PublishInput{}, err
	}

	jsonString := string(jsonBytes)

	deduplicationId := uuid.New().String()

	return sns.PublishInput{
		Message:                &jsonString,
		MessageDeduplicationId: &deduplicationId,
		MessageGroupId:         &c.DeploymentID,
		TopicArn:               &c.EventTopicARN,
	}, nil
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
