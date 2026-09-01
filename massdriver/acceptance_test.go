package massdriver

// Acceptance-test harness. TestAcc* tests drive a real `terraform` binary
// (from PATH) through plan/apply/import/destroy against a real Massdriver
// org — they create and destroy real (control-plane) objects.
//
// The framework skips them unless TF_ACC is set; run via `make testacc`,
// which loads credentials from the gitignored .env file (see .env.example).

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// testAccProviderFactories hands the framework an in-process provider, so
// tests exercise the exact code under test without a built/installed binary.
var testAccProviderFactories = map[string]func() (*schema.Provider, error){
	"massdriver": func() (*schema.Provider, error) { return Provider(), nil },
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, v := range []string{"MASSDRIVER_API_KEY", "MASSDRIVER_ORGANIZATION_ID"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests — copy .env.example to .env and run `make testacc`", v)
		}
	}
}

// testAccAPIClient builds a client from the same env credentials the provider
// under test uses, for out-of-band assertions (e.g. CheckDestroy).
func testAccAPIClient() (*ProviderClient, error) {
	return NewProviderClient(ProviderConfig{})
}

// captureID stores the named resource's current ID in *dst, for comparing
// across steps (e.g. proving a ForceNew field really replaced the object).
func captureID(name string, dst *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		*dst = rs.Primary.ID
		return nil
	}
}

// checkIDChanged asserts the named resource's ID differs from *prev — i.e.
// the previous step's change forced a replacement rather than an update.
func checkIDChanged(name string, prev *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		if rs.Primary.ID == *prev {
			return fmt.Errorf("%s ID %s is unchanged; expected a replacement", name, *prev)
		}
		return nil
	}
}

// grantImportID builds the composite `<parent_id>/<grant_id>` import ID the
// grant resources use, from the named resource's state.
func grantImportID(name, parentIDAttr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return "", fmt.Errorf("%s not found in state", name)
		}
		return rs.Primary.Attributes[parentIDAttr] + "/" + rs.Primary.ID, nil
	}
}
