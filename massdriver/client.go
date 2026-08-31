package massdriver

import (
	"errors"
	"sync"

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

	// Thunked and memoized: provisioning.NewClient() errors outside a
	// deployment, so construction waits until massdriver_resource is used.
	ProvisioningResources func() (provisioningResourcesAPI, error)

	// PlatformAuthErr is set when no API key or PAT resolved; every service
	// above is nil except InstanceAlarms. See requiresAPIKey in provider.go.
	PlatformAuthErr error

	// AlarmAuthErr is set when neither credential resolved, so InstanceAlarms
	// is nil too. See requiresAnyCredential in provider.go.
	AlarmAuthErr error
}

// ProviderConfig carries the provider block's credential settings. Empty
// fields fall back to the SDK's own resolution (environment variables, then
// the active profile in ~/.config/massdriver/config.yaml).
type ProviderConfig struct {
	APIKey         string
	OrganizationID string
	URL            string
}

// NewProviderClient builds the clients the provider needs. Three API surfaces
// take different credentials:
//
//   - Most of the GraphQL platform API: API key or PAT only.
//   - Instance alarms, also GraphQL: either credential.
//   - The REST surface behind massdriver_resource: deployment token only.
//
// An API key is used wherever it works, so a deployment token alongside it
// never shadows it. Neither credential is required to configure — a missing
// one is recorded and reported by the resource that needed it. Other
// configuration failures are fatal here, since they break every surface.
func NewProviderClient(cfg ProviderConfig) (*ProviderClient, error) {
	pc := &ProviderClient{
		ProvisioningResources: sync.OnceValues(func() (provisioningResourcesAPI, error) { return newProvisioningResources(cfg) }),
	}

	platform, err := newPlatformClient(cfg)
	if err == nil {
		pc.Config = platform.Config()
		pc.InstanceAlarms = platform.Instances
		pc.Projects = platform.Projects
		pc.Environments = platform.Environments
		pc.Components = platform.Components
		pc.ComponentLinks = platform.Components
		pc.Groups = platform.Groups
		pc.Policies = platform.Policies
		pc.Resources = platform.Resources
		pc.ResourceGrants = platform.Resources
		pc.OciRepos = platform.OciRepos
		pc.OciRepoGrants = platform.OciRepos
		return pc, nil
	}
	if !errors.Is(err, config.ErrNoCredentials) {
		return nil, err
	}

	pc.PlatformAuthErr = err

	// Alarms still work over the deployment token. That client needs an
	// organization id too — alarm queries pass one as an argument.
	alarms, alarmErr := newDeploymentPlatformClient(cfg)
	if alarmErr != nil {
		pc.AlarmAuthErr = alarmErr
		return pc, nil
	}
	pc.Config = alarms.Config()
	pc.InstanceAlarms = alarms.Instances
	return pc, nil
}

// newPlatformClient builds the GraphQL platform client from the API key or
// PAT, which is the SDK's default resolution.
func newPlatformClient(cfg ProviderConfig) (*massdriver.Client, error) {
	return massdriver.NewClient(platformOptions(cfg)...)
}

// newDeploymentPlatformClient builds a GraphQL client authenticated with the
// deployment token. Only InstanceAlarms is wired to it; the rest of the
// GraphQL API rejects deployment tokens.
func newDeploymentPlatformClient(cfg ProviderConfig) (*massdriver.Client, error) {
	return massdriver.NewClient(append(platformOptions(cfg), massdriver.WithDeploymentTokenAuth())...)
}

// platformOptions returns a fresh slice per call, so callers may append.
func platformOptions(cfg ProviderConfig) []massdriver.Option {
	opts := make([]massdriver.Option, 0, 3)
	if cfg.APIKey != "" {
		opts = append(opts, massdriver.WithAPIKey(cfg.APIKey))
	}
	if cfg.OrganizationID != "" {
		opts = append(opts, massdriver.WithOrganizationID(cfg.OrganizationID))
	}
	if cfg.URL != "" {
		opts = append(opts, massdriver.WithBaseURL(cfg.URL))
	}
	return opts
}

// newProvisioningResources builds the deployment-token REST client. The error
// is returned rather than fatal — it can't be built outside a deployment, and
// the rest of the provider must still work there.
func newProvisioningResources(cfg ProviderConfig) (provisioningResourcesAPI, error) {
	var opts []provisioning.Option
	if cfg.URL != "" {
		opts = append(opts, provisioning.WithBaseURL(cfg.URL))
	}

	client, err := provisioning.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return client.Resources, nil
}
