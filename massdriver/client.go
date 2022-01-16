package massdriver

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
)

// 95% of this is duplicated here https://github.com/massdriver-cloud/xo/blob/main/src/massdriver/main.go
// We need to move this out into a separate lib

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

func NewMassdriverClient(deployment_id, token, event_topic_arn string) (*MassdriverClient, error) {
	c := new(MassdriverClient)

	c.DeploymentID = deployment_id
	c.Token = token
	c.EventTopicARN = event_topic_arn

	awsEndpoint := os.Getenv("AWS_ENDPOINT")
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

func (c MassdriverClient) PublishEventToSNS(event *Event) error {
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return err
	}
	jsonString := string(jsonBytes)

	deduplicationId := uuid.New().String()

	input := sns.PublishInput{
		Message:                &jsonString,
		MessageDeduplicationId: &deduplicationId,
		MessageGroupId:         &c.DeploymentID,
		TopicArn:               &c.EventTopicARN,
	}

	_, err = c.SNSClient.Publish(context.Background(), &input)
	return err
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
	DeploymentId string `json:"deployment_id"`
	Artifact     string `json:"artifact"`
}

var EventTimeString = time.Now().String

func NewEvent(eventType string) *Event {
	event := new(Event)
	event.Metadata.EventType = eventType
	event.Metadata.Timestamp = EventTimeString()
	event.Metadata.Provisioner = os.Getenv("MASSDRIVER_PROVISIONER")
	return event
}
