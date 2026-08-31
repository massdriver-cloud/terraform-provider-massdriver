package massdriver

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
)

// isolateCredentialEnv clears MASSDRIVER_* from the environment and points
// XDG_CONFIG_HOME at an empty dir, so the developer's shell and real
// ~/.config/massdriver/config.yaml can't leak into a test.
func isolateCredentialEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "MASSDRIVER_") {
			// t.Setenv registers the restore; the unset is what we're after.
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// setDeploymentEnv sets every variable the platform injects into a
// provisioner container, so provisioning.NewClient can be built.
func setDeploymentEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MASSDRIVER_DEPLOYMENT_ID", "deployment-abc")
	t.Setenv("MASSDRIVER_TOKEN", "deployment-token-xyz")
	t.Setenv("MASSDRIVER_BUNDLE_NAME", "aws-vpc")
	t.Setenv("MASSDRIVER_BUNDLE_VERSION", "1.0.0")
	t.Setenv("MASSDRIVER_DEPLOYMENT_ACTION", "provision")
	t.Setenv("MASSDRIVER_INSTANCE_ID", "instance-123")
}

// A deployment token in the environment must not shadow the API key.
func TestNewProviderClientPrefersAPIKeyForPlatform(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		cfg    ProviderConfig
		want   config.AuthMethod
	}{
		{
			name:   "api key from env",
			envKey: "legacy-api-key",
			cfg:    ProviderConfig{OrganizationID: testOrgID},
			want:   config.AuthAPIKey,
		},
		{
			name: "api key from provider block",
			cfg:  ProviderConfig{APIKey: "legacy-api-key", OrganizationID: testOrgID},
			want: config.AuthAPIKey,
		},
		{
			name: "pat from provider block",
			cfg:  ProviderConfig{APIKey: "mds_personal_token", OrganizationID: testOrgID},
			want: config.AuthPAT,
		},
		{
			// The provider block wins over the env var, same as without a
			// deployment token present.
			name:   "provider block overrides env api key",
			envKey: "legacy-api-key",
			cfg:    ProviderConfig{APIKey: "mds_personal_token", OrganizationID: testOrgID},
			want:   config.AuthPAT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateCredentialEnv(t)
			setDeploymentEnv(t)
			if tt.envKey != "" {
				t.Setenv("MASSDRIVER_API_KEY", tt.envKey)
			}

			pc, err := NewProviderClient(tt.cfg)
			if err != nil {
				t.Fatalf("NewProviderClient: %v", err)
			}

			if got := pc.Config.Credentials.Method; got != tt.want {
				t.Errorf("platform auth method = %q, want %q", got, tt.want)
			}
			if pc.Config.Credentials.Secret == "deployment-token-xyz" {
				t.Error("platform client authenticated with the deployment token")
			}
		})
	}
}

// A bundle with a bare `provider "massdriver" {}` and only a deployment
// token: massdriver_resource and massdriver_instance_alarm must both work,
// and the absent API key must be recorded rather than fatal.
func TestNewProviderClientDeploymentTokenOnly(t *testing.T) {
	isolateCredentialEnv(t)
	setDeploymentEnv(t)
	t.Setenv("MASSDRIVER_ORGANIZATION_ID", testOrgID)

	pc, err := NewProviderClient(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}

	if !errors.Is(pc.PlatformAuthErr, config.ErrNoCredentials) {
		t.Errorf("PlatformAuthErr = %v, want %v", pc.PlatformAuthErr, config.ErrNoCredentials)
	}
	if pc.Projects != nil {
		t.Error("API-key-only services should be nil when no API key resolved")
	}

	if pc.AlarmAuthErr != nil {
		t.Errorf("AlarmAuthErr = %v, want nil — alarms accept a deployment token", pc.AlarmAuthErr)
	}
	if pc.InstanceAlarms == nil {
		t.Error("InstanceAlarms is nil, want a deployment-token-authenticated client")
	}
	// Alarm queries pass the organization id as an argument.
	if got := pc.Config.OrganizationID; got != testOrgID {
		t.Errorf("Config.OrganizationID = %q, want %q", got, testOrgID)
	}
	if got := pc.Config.Credentials.Method; got != config.AuthDeployment {
		t.Errorf("alarm client auth method = %q, want %q", got, config.AuthDeployment)
	}

	if _, err := pc.ProvisioningResources(); err != nil {
		t.Errorf("ProvisioningResources: %v", err)
	}
}

// Both credentials at once: every surface resolves, and the API key wins
// wherever it works — including alarms.
func TestNewProviderClientResolvesBothCredentials(t *testing.T) {
	isolateCredentialEnv(t)
	setDeploymentEnv(t)
	t.Setenv("MASSDRIVER_API_KEY", "mds_personal_token")
	// From the env, not ProviderConfig: provisioning.NewClient has no
	// organization option but still requires an org id.
	t.Setenv("MASSDRIVER_ORGANIZATION_ID", testOrgID)

	pc, err := NewProviderClient(ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}

	if pc.PlatformAuthErr != nil || pc.AlarmAuthErr != nil {
		t.Fatalf("PlatformAuthErr = %v, AlarmAuthErr = %v, want both nil", pc.PlatformAuthErr, pc.AlarmAuthErr)
	}
	if got := pc.Config.Credentials.Method; got != config.AuthPAT {
		t.Errorf("platform auth method = %q, want %q", got, config.AuthPAT)
	}
	if _, err := pc.ProvisioningResources(); err != nil {
		t.Errorf("ProvisioningResources: %v", err)
	}
}

// The provisioning client can't be built outside a deployment, but the
// provider must still configure — the error is deferred to the thunk.
func TestNewProviderClientDefersProvisioningError(t *testing.T) {
	isolateCredentialEnv(t)
	t.Setenv("MASSDRIVER_API_KEY", "mds_personal_token")

	pc, err := NewProviderClient(ProviderConfig{OrganizationID: testOrgID})
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}

	api, err := pc.ProvisioningResources()
	if err == nil {
		t.Fatalf("ProvisioningResources = %v, want an error outside a deployment", api)
	}
}

// Only a missing API key is deferred to PlatformAuthErr — other
// misconfiguration breaks every surface and must fail configuration.
func TestNewProviderClientDoesNotMaskConfigErrors(t *testing.T) {
	isolateCredentialEnv(t)
	setDeploymentEnv(t)
	t.Setenv("MASSDRIVER_API_KEY", "mds_personal_token")

	_, err := NewProviderClient(ProviderConfig{OrganizationID: testOrgID, URL: "not-a-url"})
	if err == nil {
		t.Fatal("NewProviderClient succeeded, want a URL validation error")
	}
	if !strings.Contains(err.Error(), "url must include scheme and host") {
		t.Errorf("NewProviderClient error = %v, want the URL validation error", err)
	}
}

// With neither credential available, configuration still succeeds — both
// surfaces report their own missing credential when used.
func TestNewProviderClientNoCredentials(t *testing.T) {
	isolateCredentialEnv(t)

	pc, err := NewProviderClient(ProviderConfig{OrganizationID: testOrgID})
	if err != nil {
		t.Fatalf("NewProviderClient: %v", err)
	}
	if !errors.Is(pc.PlatformAuthErr, config.ErrNoCredentials) {
		t.Errorf("PlatformAuthErr = %v, want %v", pc.PlatformAuthErr, config.ErrNoCredentials)
	}
	if pc.AlarmAuthErr == nil {
		t.Error("AlarmAuthErr is nil, want the missing-deployment-token error")
	}
	if pc.InstanceAlarms != nil {
		t.Error("InstanceAlarms should be nil when neither credential resolved")
	}
	if _, err := pc.ProvisioningResources(); err == nil {
		t.Error("ProvisioningResources succeeded, want an error outside a deployment")
	}
}
