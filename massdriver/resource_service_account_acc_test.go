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

func TestAccServiceAccount_basic(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-sa")
	var firstID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckServiceAccountDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceAccountConfig(name, "first", 60),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_service_account.test", "name", name),
					resource.TestCheckResourceAttr("massdriver_service_account.test", "description", "first"),
					resource.TestCheckResourceAttrSet("massdriver_service_account.test", "token"),
					resource.TestCheckResourceAttrSet("massdriver_service_account.test", "token_id"),
					resource.TestCheckResourceAttrPair("massdriver_service_account_group_membership.test", "service_account_id", "massdriver_service_account.test", "id"),
					resource.TestCheckResourceAttrPair("massdriver_service_account_group_membership.test", "group_id", "massdriver_group.test", "id"),
					captureID("massdriver_service_account.test", &firstID),
				),
			},
			// Name and description update in place, keeping the token.
			{
				Config: testAccServiceAccountConfig(name+"-renamed", "second", 60),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_service_account.test", "name", name+"-renamed"),
					resource.TestCheckResourceAttrPtr("massdriver_service_account.test", "id", &firstID),
					resource.TestCheckResourceAttrSet("massdriver_service_account.test", "token"),
				),
			},
			// A new expiration means a new token, so it replaces.
			{
				Config: testAccServiceAccountConfig(name+"-renamed", "second", 120),
				Check:  checkIDChanged("massdriver_service_account.test", &firstID),
			},
			// The token is only returned at creation, so import can't restore it.
			{
				ResourceName:            "massdriver_service_account.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token", "token_id", "token_expiration_minutes"},
			},
			{
				ResourceName:      "massdriver_service_account_group_membership.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccServiceAccountConfig(name, description string, expiration int) string {
	return fmt.Sprintf(`
resource "massdriver_group" "test" {
  name = %[1]q
}

resource "massdriver_service_account" "test" {
  name                     = %[1]q
  description              = %[2]q
  token_expiration_minutes = %[3]d
}

resource "massdriver_service_account_group_membership" "test" {
  group_id           = massdriver_group.test.id
  service_account_id = massdriver_service_account.test.id
}
`, name, description, expiration)
}

// Memberships die with the service account, so destroy is verified by the
// service account and group being gone.
func testAccCheckServiceAccountDestroy(s *terraform.State) error {
	pc, err := testAccAPIClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		switch rs.Type {
		case "massdriver_service_account":
			if _, err := pc.ServiceAccounts.Get(context.Background(), rs.Primary.ID); !errors.Is(err, gql.ErrNotFound) {
				return fmt.Errorf("service account %s should be destroyed; Get returned %v", rs.Primary.ID, err)
			}
		case "massdriver_group":
			if _, err := pc.Groups.Get(context.Background(), rs.Primary.ID); !errors.Is(err, gql.ErrNotFound) {
				return fmt.Errorf("group %s should be destroyed; Get returned %v", rs.Primary.ID, err)
			}
		}
	}
	return nil
}
