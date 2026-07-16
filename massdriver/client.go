package massdriver

import (
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/provisioning"
)

// ProviderClient holds one minimal interface per resource so tests can
// inject fakes without mocking the GraphQL transport. Each interface is
// declared next to its resource in resource_<name>.go.
type ProviderClient struct {
	Config config.Config

	InstanceAlarms instanceAlarmsAPI
	Projects       projectsAPI
	Environments   environmentsAPI
	Components     componentsAPI
	ComponentLinks componentLinksAPI
	Groups         groupsAPI
	Policies       policiesAPI
	Resources      resourcesAPI
	ResourceGrants resourceGrantsAPI
	OciRepos       ociReposAPI
	OciRepoGrants  ociRepoGrantsAPI

	// Thunked because provisioning.NewClient() errors when deployment env
	// vars are absent. Deferring construction lets platform-only callers
	// configure successfully and only fail if they actually use it.
	ProvisioningResources func() (provisioningResourcesAPI, error)
}

func NewProviderClient(opts ...massdriver.Option) (*ProviderClient, error) {
	platform, err := massdriver.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &ProviderClient{
		Config:         platform.Config(),
		InstanceAlarms: platform.Instances,
		Projects:       platform.Projects,
		Environments:   platform.Environments,
		Components:     platform.Components,
		ComponentLinks: platform.Components,
		Groups:         platform.Groups,
		Policies:       platform.Policies,
		Resources:      platform.Resources,
		ResourceGrants: platform.Resources,
		OciRepos:       platform.OciRepos,
		OciRepoGrants:  platform.OciRepos,
		ProvisioningResources: func() (provisioningResourcesAPI, error) {
			prov, err := provisioning.NewClient()
			if err != nil {
				return nil, err
			}
			return prov.Resources, nil
		},
	}, nil
}
