package massdriver

import (
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/provisioning"
)

// ProviderClient is the provider's wrapper around the Massdriver SDK. Each
// resource has its own minimal interface field (declared next to the resource
// in resource_<name>.go) so tests inject fakes that return SDK domain types
// directly — no GraphQL transport mocking.
//
// Production wiring lives in NewProviderClient: platform services come from
// *massdriver.Client, the REST `massdriver_resource` service comes from
// *provisioning.Client. Resource code doesn't know or care which physical
// client a service was pulled from; the interface fields are the boundary.
type ProviderClient struct {
	// Config is the resolved platform-client configuration (organization ID,
	// base URL, credentials method, etc.). Resources that need the org ID
	// (e.g. to prefix bare resource-type refs) read it here.
	Config config.Config

	// Service interface fields — each declared in the resource file that
	// uses it. Wired below from *massdriver.Client (platform GraphQL).
	InstanceAlarms instanceAlarmsAPI
	Projects       projectsAPI
	Environments   environmentsAPI
	Components     componentsAPI
	Groups         groupsAPI
	Policies       policiesAPI
	Resources      resourcesAPI
	OciRepos       ociReposAPI

	// ProvisioningResources serves massdriver_resource only. It's a thunk
	// rather than a value because provisioning.NewClient() errors outside
	// a bundle deployment — we don't want platform-only users to hit a
	// configure-time failure. The error is deferred until the resource
	// actually tries to use it, at which point we surface a clear message.
	ProvisioningResources func() (provisioningResourcesAPI, error)
}

// NewProviderClient wires production services. The platform client serves
// every resource except massdriver_resource; the provisioning client serves
// massdriver_resource. The two clients have different auth models — platform
// takes PATs / service accounts, provisioning takes a deployment token —
// hence the lazy construction of provisioning to keep platform-only callers
// working outside a deployment.
func NewProviderClient() (*ProviderClient, error) {
	platform, err := massdriver.NewClient()
	if err != nil {
		return nil, err
	}

	return &ProviderClient{
		Config:         platform.Config(),
		InstanceAlarms: platform.Instances,
		Projects:       platform.Projects,
		Environments:   platform.Environments,
		Components:     platform.Components,
		Groups:         platform.Groups,
		Policies:       platform.Policies,
		Resources:      platform.Resources,
		OciRepos:       platform.OciRepos,
		ProvisioningResources: func() (provisioningResourcesAPI, error) {
			prov, err := provisioning.NewClient()
			if err != nil {
				return nil, err
			}
			return prov.Resources, nil
		},
	}, nil
}

