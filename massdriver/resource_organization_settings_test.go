package massdriver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/organizations"
)

// fakeOrganizationSettings applies updates the way the API does: only the
// fields set on the input change.
type fakeOrganizationSettings struct {
	mu       sync.Mutex
	settings organizations.Settings
	getErr   error
	updates  []organizations.UpdateSettingsInput
}

func (f *fakeOrganizationSettings) GetSettings(_ context.Context) (*organizations.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	s := f.settings
	return &s, nil
}

func (f *fakeOrganizationSettings) UpdateSettings(_ context.Context, input organizations.UpdateSettingsInput) (*organizations.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, input)
	if input.DefaultBundleAccess != "" {
		f.settings.DefaultBundleAccess = input.DefaultBundleAccess
	}
	if input.NamingConvention != nil {
		f.settings.NamingConvention = *input.NamingConvention
	}
	if input.DefaultTags != nil {
		f.settings.DefaultTags = slices.Clone(*input.DefaultTags)
	}
	s := f.settings
	return &s, nil
}

func (f *fakeOrganizationSettings) lastUpdate() (organizations.UpdateSettingsInput, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) == 0 {
		return organizations.UpdateSettingsInput{}, 0
	}
	return f.updates[len(f.updates)-1], len(f.updates)
}

func fakeProviderFactories(pc *ProviderClient) map[string]func() (*schema.Provider, error) {
	return map[string]func() (*schema.Provider, error){
		"massdriver": func() (*schema.Provider, error) {
			p := Provider()
			p.ConfigureContextFunc = func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) { return pc, nil }
			return p, nil
		},
	}
}

func TestResourceOrganizationSettingsRead(t *testing.T) {
	fake := &fakeOrganizationSettings{settings: organizations.Settings{
		DefaultBundleAccess: organizations.DefaultBundleAccessAllProjects,
		NamingConvention:    "{{attrs.team}}-{{instance.id}}",
		DefaultTags:         []string{"team", "cost_center"},
	}}
	pc := &ProviderClient{OrganizationSettings: fake}

	rd := resourceOrganizationSettings().TestResourceData()
	rd.SetId("acme")

	if diags := resourceOrganizationSettingsRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("grant_new_bundles_to_all_projects") != true {
		t.Error("grant_new_bundles_to_all_projects should be true for ALL_PROJECTS")
	}
	if rd.Get("naming_convention") != "{{attrs.team}}-{{instance.id}}" {
		t.Errorf("got naming_convention %v", rd.Get("naming_convention"))
	}
	tags := expandStringList(rd.Get("default_tags").(*schema.Set).List())
	slices.Sort(tags)
	if !slices.Equal(tags, []string{"cost_center", "team"}) {
		t.Errorf("got default_tags %v", tags)
	}
}

func TestResourceOrganizationSettingsReadNotFoundClearsID(t *testing.T) {
	pc := &ProviderClient{OrganizationSettings: &fakeOrganizationSettings{getErr: fmt.Errorf("get organization settings: %w", gql.ErrNotFound)}}

	rd := resourceOrganizationSettings().TestResourceData()
	rd.SetId("acme")

	if diags := resourceOrganizationSettingsRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("got id %q, want it cleared", rd.Id())
	}
}

func TestResourceOrganizationSettingsReadPropagatesForbidden(t *testing.T) {
	pc := &ProviderClient{OrganizationSettings: &fakeOrganizationSettings{getErr: errors.New("forbidden")}}

	rd := resourceOrganizationSettings().TestResourceData()
	rd.SetId("acme")

	if diags := resourceOrganizationSettingsRead(t.Context(), rd, pc); !diags.HasError() {
		t.Fatal("got no error, want forbidden")
	}
}

func TestResourceOrganizationSettingsImportRejectsOtherOrg(t *testing.T) {
	pc := &ProviderClient{Config: config.Config{OrganizationID: "acme"}}

	rd := resourceOrganizationSettings().TestResourceData()
	rd.SetId("other-org")

	if _, err := resourceOrganizationSettingsImport(t.Context(), rd, pc); err == nil {
		t.Fatal("got no error importing another organization's settings")
	}

	rd.SetId("acme")
	if _, err := resourceOrganizationSettingsImport(t.Context(), rd, pc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Drives the real terraform binary against a fake API to check which
// settings each plan actually sends, including explicit resets that
// Optional+Computed attributes would otherwise swallow.
func TestResourceOrganizationSettingsLifecycle(t *testing.T) {
	fake := &fakeOrganizationSettings{settings: organizations.Settings{
		DefaultBundleAccess: organizations.DefaultBundleAccessNone,
		NamingConvention:    "{{org.id}}-{{instance.id}}",
		DefaultTags:         []string{"team"},
	}}
	pc := &ProviderClient{Config: config.Config{OrganizationID: "acme"}, OrganizationSettings: fake}
	const addr = "massdriver_organization_settings.this"

	expectUpdate := func(n int, check func(organizations.UpdateSettingsInput) error) resource.TestCheckFunc {
		return func(*terraform.State) error {
			in, count := fake.lastUpdate()
			if count != n {
				return fmt.Errorf("got %d updates, want %d", count, n)
			}
			return check(in)
		}
	}

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: fakeProviderFactories(pc),
		Steps: []resource.TestStep{
			{
				Config: `
resource "massdriver_organization_settings" "this" {
  grant_new_bundles_to_all_projects = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "acme"),
					resource.TestCheckResourceAttr(addr, "grant_new_bundles_to_all_projects", "true"),
					resource.TestCheckResourceAttr(addr, "naming_convention", "{{org.id}}-{{instance.id}}"),
					resource.TestCheckTypeSetElemAttr(addr, "default_tags.*", "team"),
					expectUpdate(1, func(in organizations.UpdateSettingsInput) error {
						if in.DefaultBundleAccess != organizations.DefaultBundleAccessAllProjects || in.NamingConvention != nil || in.DefaultTags != nil {
							return fmt.Errorf("create sent %+v, want only default bundle access", in)
						}
						return nil
					}),
				),
			},
			{
				Config: `
resource "massdriver_organization_settings" "this" {
  grant_new_bundles_to_all_projects = true
  naming_convention                 = "{{attrs.team}}-{{instance.id}}"
  default_tags                      = ["team", "cost_center"]
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "naming_convention", "{{attrs.team}}-{{instance.id}}"),
					resource.TestCheckResourceAttr(addr, "default_tags.#", "2"),
					expectUpdate(2, func(in organizations.UpdateSettingsInput) error {
						if in.DefaultBundleAccess != organizations.DefaultBundleAccessAllProjects || in.NamingConvention == nil || in.DefaultTags == nil {
							return fmt.Errorf("update sent %+v, want every configured setting", in)
						}
						return nil
					}),
				),
			},
			{
				Config: `
resource "massdriver_organization_settings" "this" {
  grant_new_bundles_to_all_projects = false
  naming_convention                 = ""
  default_tags                      = []
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "grant_new_bundles_to_all_projects", "false"),
					resource.TestCheckResourceAttr(addr, "naming_convention", ""),
					resource.TestCheckResourceAttr(addr, "default_tags.#", "0"),
					expectUpdate(3, func(in organizations.UpdateSettingsInput) error {
						if in.DefaultBundleAccess != organizations.DefaultBundleAccessNone {
							return fmt.Errorf("got default bundle access %q, want NONE", in.DefaultBundleAccess)
						}
						if in.NamingConvention == nil || *in.NamingConvention != "" {
							return fmt.Errorf("got naming convention %v, want a reset to \"\"", in.NamingConvention)
						}
						if in.DefaultTags == nil || len(*in.DefaultTags) != 0 {
							return fmt.Errorf("got default tags %v, want a reset to []", in.DefaultTags)
						}
						return nil
					}),
				),
			},
			{
				Config: `
resource "massdriver_organization_settings" "this" {
  naming_convention = "{{org.id}}-{{instance.id}}"
}`,
				Check: resource.TestCheckResourceAttr(addr, "naming_convention", "{{org.id}}-{{instance.id}}"),
			},
			// A reset to "" on its own must still plan a change.
			{
				Config: `
resource "massdriver_organization_settings" "this" {
  naming_convention = ""
}`,
				Check: expectUpdate(5, func(in organizations.UpdateSettingsInput) error {
					if in.NamingConvention == nil || *in.NamingConvention != "" || in.DefaultBundleAccess != "" || in.DefaultTags != nil {
						return fmt.Errorf("got %+v, want only a naming convention reset", in)
					}
					return nil
				}),
			},
			// Dropping arguments stops managing them; nothing is sent.
			{
				Config: `
resource "massdriver_organization_settings" "this" {}
`,
				Check: expectUpdate(5, func(organizations.UpdateSettingsInput) error { return nil }),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "acme",
				ImportStateVerify: true,
			},
		},
		// Destroy only forgets the settings.
		CheckDestroy: func(*terraform.State) error {
			if _, n := fake.lastUpdate(); n != 5 {
				return fmt.Errorf("destroy sent updates: got %d total, want 5", n)
			}
			return nil
		},
	})
}
