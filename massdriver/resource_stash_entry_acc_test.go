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

func TestAccStashEntry_basic(t *testing.T) {
	project := "tfacc" + acctest.RandString(10)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckStashEntryDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccStashEntryConfig(project, `
  value = "db.example.com"
  label = "Database host"
`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "id", "project:"+project+"/db/hostname"),
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "value", "db.example.com"),
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "label", "Database host"),
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "format", "TEXT"),
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "sensitive", "false"),
				),
			},
			// A JSON value with jsonencode, read back through both data sources.
			{
				Config: testAccStashEntryConfig(project, `
  format = "JSON"
  value  = jsonencode({ host = "db.example.com", port = 5432 })
`) + testAccStashEntryDataSources(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "format", "JSON"),
					resource.TestCheckResourceAttr("data.massdriver_stash_entry.test", "data", `{"host":"db.example.com","port":5432}`),
					resource.TestCheckResourceAttr("data.massdriver_stash_entries.test", "keys.#", "1"),
					resource.TestCheckResourceAttr("data.massdriver_stash_entries.test", "keys.0", "db/hostname"),
				),
			},
			// An immutable entry is written again on change. An in-place
			// write would be refused by the server.
			{
				Config: testAccStashEntryConfig(project, `
  value     = "v1"
  immutable = true
`),
				Check: resource.TestCheckResourceAttr("massdriver_stash_entry.test", "immutable", "true"),
			},
			{
				Config: testAccStashEntryConfig(project, `
  value     = "v2"
  immutable = true
`),
				Check: resource.TestCheckResourceAttr("massdriver_stash_entry.test", "value", "v2"),
			},
			// A sensitive value the API key can read back.
			{
				Config: testAccStashEntryConfig(project, `
  sensitive_value = "s3cret"
`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "sensitive", "true"),
					resource.TestCheckResourceAttr("massdriver_stash_entry.test", "value", ""),
				),
			},
			{
				ResourceName:            "massdriver_stash_entry.test",
				ImportState:             true,
				ImportStateId:           "project:" + project + "/db/hostname",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ttl"},
			},
		},
	})
}

func testAccStashEntryConfig(project, body string) string {
	return fmt.Sprintf(`
resource "massdriver_project" "test" {
  identifier = %[1]q
}

resource "massdriver_stash_entry" "test" {
  project = massdriver_project.test.identifier
  key     = "db/hostname"
%[2]s}
`, project, body)
}

func testAccStashEntryDataSources() string {
	return `
data "massdriver_stash_entry" "test" {
  project    = massdriver_stash_entry.test.project
  key        = massdriver_stash_entry.test.key
  depends_on = [massdriver_stash_entry.test]
}

data "massdriver_stash_entries" "test" {
  project    = massdriver_stash_entry.test.project
  key_prefix = "db/"
  depends_on = [massdriver_stash_entry.test]
}
`
}

func testAccCheckStashEntryDestroy(s *terraform.State) error {
	pc, err := testAccAPIClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "massdriver_stash_entry" {
			continue
		}
		scope, key, err := parseStashEntryID(rs.Primary.ID)
		if err != nil {
			return err
		}
		_, err = pc.Stash.Get(context.Background(), scope, key)
		if errors.Is(err, gql.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("stash entry %s should be destroyed", rs.Primary.ID)
	}
	return nil
}
