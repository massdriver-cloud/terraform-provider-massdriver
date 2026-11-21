package massdriver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/organizations"
)

func TestAccCustomAttribute_basic(t *testing.T) {
	key := strings.ReplaceAll(acctest.RandomWithPrefix("tf_acc_attr"), "-", "_")
	var firstID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckCustomAttributeDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCustomAttributeConfig(key, "PROJECT", `["a", "b"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_custom_attribute.test", "key", key),
					resource.TestCheckResourceAttr("massdriver_custom_attribute.test", "scope", "PROJECT"),
					resource.TestCheckResourceAttr("massdriver_custom_attribute.test", "required", "false"),
					resource.TestCheckResourceAttr("massdriver_custom_attribute.test", "values.#", "2"),
					captureID("massdriver_custom_attribute.test", &firstID),
				),
			},
			// Values update in place.
			{
				Config: testAccCustomAttributeConfig(key, "PROJECT", `["a", "b", "c"]`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_custom_attribute.test", "values.#", "3"),
					resource.TestCheckResourceAttrPtr("massdriver_custom_attribute.test", "id", &firstID),
				),
			},
			// Scope is immutable, so it replaces.
			{
				Config: testAccCustomAttributeConfig(key, "ENVIRONMENT", `["a", "b", "c"]`),
				Check:  checkIDChanged("massdriver_custom_attribute.test", &firstID),
			},
			{
				ResourceName:      "massdriver_custom_attribute.test",
				ImportState:       true,
				ImportStateId:     key,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCustomAttributeConfig(key, scope, values string) string {
	return fmt.Sprintf(`
resource "massdriver_custom_attribute" "test" {
  key    = %q
  scope  = %q
  values = %s
}
`, key, scope, values)
}

func testAccCheckCustomAttributeDestroy(s *terraform.State) error {
	pc, err := testAccAPIClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "massdriver_custom_attribute" {
			continue
		}
		attr, err := findCustomAttribute(context.Background(), pc.CustomAttributes, func(a organizations.CustomAttribute) bool { return a.ID == rs.Primary.ID })
		if err != nil {
			return err
		}
		if attr != nil {
			return fmt.Errorf("custom attribute %s should be destroyed", rs.Primary.ID)
		}
	}
	return nil
}
