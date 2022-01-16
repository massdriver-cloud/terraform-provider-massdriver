package massdriver

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var testAccProviders map[string]*schema.Provider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]*schema.Provider{
		"massdriver": testAccProvider,
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = Provider()
}

func testAccPreCheck(t *testing.T) {
	if err := os.Getenv("AWS_ENDPOINT"); err == "" {
		t.Fatal("AWS_ENDPOINT must be set for acceptance tests (requires localstack)")
	}
	if err := os.Getenv("MASSDRIVER_DEPLOYMENT_ID"); err == "" {
		t.Fatal("MASSDRIVER_DEPLOYMENT_ID must be set for acceptance tests")
	}
	if err := os.Getenv("MASSDRIVER_TOKEN"); err == "" {
		t.Fatal("MASSDRIVER_TOKEN must be set for acceptance tests")
	}
	if err := os.Getenv("MASSDRIVER_EVENT_TOPIC_ARN"); err == "" {
		t.Fatal("MASSDRIVER_EVENT_TOPIC_ARN must be set for acceptance tests")
	}
}
