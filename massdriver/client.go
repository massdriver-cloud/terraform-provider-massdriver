package massdriver

import (
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/artifacts"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/packagealarms"
)

type ProviderClient struct {
	Client *client.Client
}

func NewProviderClient() (*ProviderClient, error) {
	client, err := client.New()
	if err != nil {
		return nil, err
	}
	return &ProviderClient{
		Client: client,
	}, nil
}

func (p *ProviderClient) ArtifactService() *artifacts.Service {
	return artifacts.NewService(p.Client)
}

func (p *ProviderClient) PackageAlarmsService() *packagealarms.Service {
	return packagealarms.NewService(p.Client)
}
