package massdriver

import (
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/client"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	"terraform-provider-massdriver/internal/gqlmock"
)

const testOrgID = "test-org"

// newMockProvider returns a *ProviderClient backed by a gqlmock.Recorder.
// The recorder dispatches different responses by genqlient operation name —
// keys are operation names like "createProject", "getProject", and values
// are JSON-shaped maps with a top-level "data" (and optionally "errors") key.
func newMockProvider(responses map[string]map[string]any) (*ProviderClient, *gqlmock.Recorder) {
	rec := gqlmock.NewClientWithResponses(responses)
	return &ProviderClient{
		Client: &client.Client{
			Config: config.Config{OrganizationID: testOrgID},
			GQLv1:  rec,
		},
	}, rec
}
