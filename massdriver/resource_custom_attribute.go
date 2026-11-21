package massdriver

import (
	"context"
	"fmt"
	"iter"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/organizations"
)

// customAttributesAPI is the slice of *organizations.Service this resource
// calls.
type customAttributesAPI interface {
	IterCustomAttributes(ctx context.Context, input organizations.ListCustomAttributesInput) iter.Seq2[organizations.CustomAttribute, error]
	CreateCustomAttribute(ctx context.Context, input organizations.CreateCustomAttributeInput) (*organizations.CustomAttribute, error)
	UpdateCustomAttribute(ctx context.Context, id string, input organizations.UpdateCustomAttributeInput) (*organizations.CustomAttribute, error)
	DeleteCustomAttribute(ctx context.Context, id string) (*organizations.CustomAttribute, error)
}

var _ customAttributesAPI = (*organizations.Service)(nil)

var customAttributeKeyFormat = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

var customAttributeScopes = []string{
	string(organizations.AttributeScopeProject),
	string(organizations.AttributeScopeEnvironment),
	string(organizations.AttributeScopeComponent),
	string(organizations.AttributeScopeRepo),
}

func resourceCustomAttribute() *schema.Resource {
	return &schema.Resource{
		Description: "A custom attribute declared for the organization. Custom attributes are the keys (with a closed set of allowed values) that projects, environments, components, and bundle repositories are tagged with, and that group policy conditions and default tags reference. " +
			"Deleting a custom attribute does not remove values already set on resources.",

		CreateContext: resourceCustomAttributeCreate,
		ReadContext:   resourceCustomAttributeRead,
		UpdateContext: resourceCustomAttributeUpdate,
		DeleteContext: resourceCustomAttributeDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceCustomAttributeImport,
		},

		Schema: map[string]*schema.Schema{
			"key": {
				Description: "Attribute key. 1-64 characters, starting with a letter or underscore and containing only letters, digits, and underscores. Unique within the organization, case-insensitively. Changing it replaces the attribute, including a change only in letter case, so when importing, match the casing the attribute has in the organization.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				ValidateFunc: validation.StringMatch(customAttributeKeyFormat,
					"must be 1-64 characters, start with a letter or underscore, and contain only letters, digits, and underscores"),
			},
			"scope": {
				Description:  "Where the attribute is set: `PROJECT`, `ENVIRONMENT`, `COMPONENT`, or `REPO`. Project and environment values cascade to everything beneath them. Changing it replaces the attribute.",
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(customAttributeScopes, false),
			},
			"values": {
				Description: "The closed set of values the attribute may take. At least one; no duplicates, empty strings, or `*`. Changing the set does not revalidate resources already tagged.",
				Type:        schema.TypeList,
				Required:    true,
				MinItems:    1,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.All(validation.StringIsNotEmpty, validation.StringNotInSlice([]string{"*"}, false)),
				},
			},
			"required": {
				Description: "Whether every new resource at `scope` must set this attribute. Defaults to `false` (the platform's own default is `true`). Existing resources are not revalidated.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
		},
	}
}

func resourceCustomAttributeCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	required := d.Get("required").(bool)
	attr, err := pc.CustomAttributes.CreateCustomAttribute(ctx, organizations.CreateCustomAttributeInput{
		Key:      d.Get("key").(string),
		Scope:    organizations.AttributeScope(d.Get("scope").(string)),
		Required: &required,
		Values:   expandStringList(d.Get("values").([]any)),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(attr.ID)
	setCustomAttribute(d, attr)
	return nil
}

func resourceCustomAttributeRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	attr, err := findCustomAttribute(ctx, pc.CustomAttributes, func(a organizations.CustomAttribute) bool { return a.ID == d.Id() })
	if err != nil {
		return diag.FromErr(err)
	}
	if attr == nil {
		d.SetId("")
		return nil
	}

	setCustomAttribute(d, attr)
	return nil
}

func resourceCustomAttributeUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	required := d.Get("required").(bool)
	attr, err := pc.CustomAttributes.UpdateCustomAttribute(ctx, d.Id(), organizations.UpdateCustomAttributeInput{
		Required: &required,
		Values:   expandStringList(d.Get("values").([]any)),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	setCustomAttribute(d, attr)
	return nil
}

func resourceCustomAttributeDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.CustomAttributes.DeleteCustomAttribute(ctx, d.Id()); err != nil {
		// Delete reports a missing attribute as a failed mutation rather than
		// a not-found, so check whether it's already gone.
		attr, findErr := findCustomAttribute(ctx, pc.CustomAttributes, func(a organizations.CustomAttribute) bool { return a.ID == d.Id() })
		if findErr != nil || attr != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return nil
}

// resourceCustomAttributeImport accepts the attribute's ID or its key.
func resourceCustomAttributeImport(ctx context.Context, d *schema.ResourceData, meta any) ([]*schema.ResourceData, error) {
	pc := meta.(*ProviderClient)
	id := d.Id()

	attr, err := findCustomAttribute(ctx, pc.CustomAttributes, func(a organizations.CustomAttribute) bool {
		return a.ID == id || strings.EqualFold(a.Key, id)
	})
	if err != nil {
		return nil, err
	}
	if attr == nil {
		return nil, fmt.Errorf("no custom attribute with ID or key %q", id)
	}

	d.SetId(attr.ID)
	return []*schema.ResourceData{d}, nil
}

// findCustomAttribute scans the organization's custom attributes, since the
// API has no lookup by ID. It returns nil when none match.
func findCustomAttribute(ctx context.Context, api customAttributesAPI, match func(organizations.CustomAttribute) bool) (*organizations.CustomAttribute, error) {
	for attr, err := range api.IterCustomAttributes(ctx, organizations.ListCustomAttributesInput{PageSize: 100}) {
		if err != nil {
			return nil, err
		}
		if match(attr) {
			return &attr, nil
		}
	}
	return nil, nil
}

func setCustomAttribute(d *schema.ResourceData, attr *organizations.CustomAttribute) {
	d.Set("key", attr.Key)
	d.Set("scope", attr.Scope)
	d.Set("values", attr.Values)
	d.Set("required", attr.Required)
}

func expandStringList(raw []any) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}
