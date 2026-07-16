package massdriver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
)

func TestAccOciRepositoryGrant_basic(t *testing.T) {
	repoName := acctest.RandomWithPrefix("tf-acc-grant")
	var firstGrantID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckOciRepositoryGrantDestroy,
		Steps: []resource.TestStep{
			// Conditional grant.
			{
				Config: testAccOciRepositoryGrantConfig(repoName, `jsonencode({ team = ["tf-acc"] })`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_oci_repository.test", "name", repoName),
					resource.TestCheckResourceAttr("massdriver_oci_repository_grant.test", "recipient_conditions", `{"team":["tf-acc"]}`),
					resource.TestCheckResourceAttrPair("massdriver_oci_repository_grant.test", "repository_id", "massdriver_oci_repository.test", "id"),
					captureID("massdriver_oci_repository_grant.test", &firstGrantID),
				),
			},
			// Grants are immutable: changing conditions must replace (new ID).
			{
				Config: testAccOciRepositoryGrantConfig(repoName, `"*"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_oci_repository_grant.test", "recipient_conditions", "*"),
					checkIDChanged("massdriver_oci_repository_grant.test", &firstGrantID),
				),
			},
			// Import round-trips via the composite <repository_id>/<grant_id>.
			{
				ResourceName:      "massdriver_oci_repository_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: grantImportID("massdriver_oci_repository_grant.test", "repository_id"),
			},
		},
	})
}

func testAccOciRepositoryGrantConfig(repoName, conditions string) string {
	return fmt.Sprintf(`
resource "massdriver_oci_repository" "test" {
  name          = %[1]q
  artifact_type = "BUNDLE"
  attributes = {
    team = "tf-acc"
  }
}

resource "massdriver_oci_repository_grant" "test" {
  repository_id        = massdriver_oci_repository.test.id
  recipient_conditions = %[2]s
}
`, repoName, conditions)
}

// The grant dies with its parent, so destroy is verified by the repository
// being gone (grants have no get-by-ID API to probe directly).
func testAccCheckOciRepositoryGrantDestroy(s *terraform.State) error {
	pc, err := testAccAPIClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "massdriver_oci_repository" {
			continue
		}
		if _, err := pc.OciRepos.Get(context.Background(), rs.Primary.ID); !errors.Is(err, gql.ErrNotFound) {
			return fmt.Errorf("oci repository %s should be destroyed; Get returned %v", rs.Primary.ID, err)
		}
	}
	return nil
}
