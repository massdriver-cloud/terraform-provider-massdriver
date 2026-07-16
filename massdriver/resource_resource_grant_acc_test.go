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

func TestAccResourceGrant_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-grant")
	var firstGrantID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckResourceGrantDestroy,
		Steps: []resource.TestStep{
			// Conditional grant.
			{
				Config: testAccResourceGrantConfig(name, `jsonencode({ team = ["tf-acc"] })`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_imported_resource.test", "name", name),
					resource.TestCheckResourceAttr("massdriver_resource_grant.test", "recipient_conditions", `{"team":["tf-acc"]}`),
					resource.TestCheckResourceAttrPair("massdriver_resource_grant.test", "resource_id", "massdriver_imported_resource.test", "id"),
					captureID("massdriver_resource_grant.test", &firstGrantID),
				),
			},
			// Grants are immutable: changing conditions must replace (new ID).
			{
				Config: testAccResourceGrantConfig(name, `"*"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_resource_grant.test", "recipient_conditions", "*"),
					checkIDChanged("massdriver_resource_grant.test", &firstGrantID),
				),
			},
			// Import round-trips via the composite <resource_id>/<grant_id>.
			{
				ResourceName:      "massdriver_resource_grant.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: grantImportID("massdriver_resource_grant.test", "resource_id"),
			},
		},
	})
}

func testAccResourceGrantConfig(name, conditions string) string {
	return fmt.Sprintf(`
resource "massdriver_imported_resource" "test" {
  name          = %[1]q
  resource_type = "aws-iam-role"
  resource = jsonencode({
    data  = { arn = "arn:aws:iam::111111111111:role/%[1]s" }
    specs = { aws = { region = "us-west-2" } }
  })
}

resource "massdriver_resource_grant" "test" {
  resource_id          = massdriver_imported_resource.test.id
  recipient_conditions = %[2]s
}
`, name, conditions)
}

// The grant dies with its parent, so destroy is verified by the resource
// being gone (grants have no get-by-ID API to probe directly).
func testAccCheckResourceGrantDestroy(s *terraform.State) error {
	pc, err := testAccAPIClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "massdriver_imported_resource" {
			continue
		}
		if _, err := pc.Resources.Get(context.Background(), rs.Primary.ID); !errors.Is(err, gql.ErrNotFound) {
			return fmt.Errorf("imported resource %s should be destroyed; Get returned %v", rs.Primary.ID, err)
		}
	}
	return nil
}
