package massdriver

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/organizations"
)

// organizationSettingsAPI is the slice of *organizations.Service this
// resource calls.
type organizationSettingsAPI interface {
	GetSettings(ctx context.Context) (*organizations.Settings, error)
	UpdateSettings(ctx context.Context, input organizations.UpdateSettingsInput) (*organizations.Settings, error)
}

var _ organizationSettingsAPI = (*organizations.Service)(nil)

func resourceOrganizationSettings() *schema.Resource {
	return &schema.Resource{
		Description: "Organization-wide settings for the provider's configured organization. Declare at most one. " +
			"Only the arguments you set are managed: an omitted argument keeps whatever value the organization already has. " +
			"Destroying this resource stops managing the settings without changing them. Requires the `organization:manageSettings` action (organization admins).",

		CreateContext: resourceOrganizationSettingsCreate,
		ReadContext:   resourceOrganizationSettingsRead,
		UpdateContext: resourceOrganizationSettingsUpdate,
		DeleteContext: resourceOrganizationSettingsDelete,
		CustomizeDiff: resourceOrganizationSettingsCustomizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: resourceOrganizationSettingsImport,
		},

		Schema: map[string]*schema.Schema{
			"grant_new_bundles_to_all_projects": {
				Description: "When `true`, each new bundle repository is automatically granted `repo:pull` to every project; when `false`, new repositories stay restricted until a grant is authored. Only affects repositories created after the change.",
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
			},
			"naming_convention": {
				Description: "Liquid template that names every provisioned cloud resource, e.g. `{{attrs.team}}-{{instance.id}}`. " +
					"Atoms include `{{org.id}}`, `{{project.id}}`, `{{environment.id}}`, `{{instance.id}}`, `{{component.suffix}}`, and `{{attrs.KEY}}` for any declared custom attribute. " +
					"Set to `\"\"` to restore the default `{{instance.id}}-{{component.suffix}}`. The resolved name is fixed on each instance when it is first created; changing the convention does not rename existing instances.",
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.StringLenBetween(0, 255),
			},
			"default_tags": {
				Description: "Custom attribute keys whose values are sent to deployments as default cloud tags. Each key must apply to instances. `managed-by` and `md-instance` are always sent. " +
					"Set to `[]` to send only those. Once an organization has chosen its default tags it cannot return to the legacy tag set, so removing this argument leaves the current tags in place.",
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.StringIsNotEmpty,
				},
			},
		},
	}
}

func resourceOrganizationSettingsCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := updateOrganizationSettings(ctx, pc, organizationSettingsInput(d)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(pc.Config.OrganizationID)
	return resourceOrganizationSettingsRead(ctx, d, meta)
}

func resourceOrganizationSettingsRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	settings, err := pc.OrganizationSettings.GetSettings(ctx)
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("grant_new_bundles_to_all_projects", settings.DefaultBundleAccess == organizations.DefaultBundleAccessAllProjects)
	d.Set("naming_convention", settings.NamingConvention)
	d.Set("default_tags", settings.DefaultTags)
	return nil
}

func resourceOrganizationSettingsUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if err := updateOrganizationSettings(ctx, pc, organizationSettingsInput(d)); err != nil {
		return diag.FromErr(err)
	}

	return resourceOrganizationSettingsRead(ctx, d, meta)
}

func resourceOrganizationSettingsDelete(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
	d.SetId("")
	return nil
}

func resourceOrganizationSettingsImport(_ context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
	pc := meta.(*ProviderClient)

	if d.Id() != pc.Config.OrganizationID {
		return nil, fmt.Errorf("organization settings can only be imported for the provider's organization %q, got %q", pc.Config.OrganizationID, d.Id())
	}
	return []*schema.ResourceData{d}, nil
}

// resourceOrganizationSettingsCustomizeDiff plans a reset when the config
// explicitly sets `naming_convention = ""` or `default_tags = []`. Both are
// Optional+Computed, so the SDK would otherwise read an empty value as unset
// and keep the current one. The SDK also drops a SetNew to "" on a computed
// string, so the naming convention is planned as known-after-apply instead.
func resourceOrganizationSettingsCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	raw := d.GetRawConfig()

	if configured(raw, "naming_convention") && raw.GetAttr("naming_convention").AsString() == "" {
		if old, _ := d.GetChange("naming_convention"); old.(string) != "" {
			if err := d.SetNewComputed("naming_convention"); err != nil {
				return err
			}
		}
	}

	if configured(raw, "default_tags") && raw.GetAttr("default_tags").LengthInt() == 0 {
		if old, _ := d.GetChange("default_tags"); old.(*schema.Set).Len() > 0 {
			if err := d.SetNew("default_tags", []any{}); err != nil {
				return err
			}
		}
	}

	return nil
}

// organizationSettingsInput sends every setting the config sets, so settings
// left out of the config are never touched.
func organizationSettingsInput(d *schema.ResourceData) organizations.UpdateSettingsInput {
	var input organizations.UpdateSettingsInput
	raw := d.GetRawConfig()
	include := func(key string) bool { return configured(raw, key) }

	if include("grant_new_bundles_to_all_projects") {
		input.DefaultBundleAccess = organizations.DefaultBundleAccessNone
		if d.Get("grant_new_bundles_to_all_projects").(bool) {
			input.DefaultBundleAccess = organizations.DefaultBundleAccessAllProjects
		}
	}
	if include("naming_convention") {
		// Read from config: a reset is planned as unknown, and d.Get falls
		// back to the old value for an unknown attribute.
		convention := raw.GetAttr("naming_convention").AsString()
		input.NamingConvention = &convention
	}
	if include("default_tags") {
		tags := expandStringList(d.Get("default_tags").(*schema.Set).List())
		input.DefaultTags = &tags
	}

	return input
}

func updateOrganizationSettings(ctx context.Context, pc *ProviderClient, input organizations.UpdateSettingsInput) error {
	if input.DefaultBundleAccess == "" && input.NamingConvention == nil && input.DefaultTags == nil {
		return nil
	}
	_, err := pc.OrganizationSettings.UpdateSettings(ctx, input)
	return err
}

// configured reports whether the config sets key to a known, non-null value.
func configured(raw cty.Value, key string) bool {
	if !raw.IsKnown() || raw.IsNull() || !raw.Type().IsObjectType() || !raw.Type().HasAttribute(key) {
		return false
	}
	v := raw.GetAttr(key)
	return v.IsKnown() && !v.IsNull()
}
