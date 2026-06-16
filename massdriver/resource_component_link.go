package massdriver

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/components"
)

// componentLinksAPI is the slice of *components.Service used by this resource.
// The SDK has no GetLink and no per-link refresh — Read instead fetches the
// parent project (which the SDK returns with its Links populated) and walks
// the list to confirm existence.
type componentLinksAPI interface {
	AddLink(ctx context.Context, input components.AddLinkInput) (*components.Link, error)
	RemoveLink(ctx context.Context, linkID string) (*components.Link, error)
}

var _ componentLinksAPI = (*components.Service)(nil)

// suppressAfterCreate returns true once the resource has an ID, suppressing
// any subsequent diff. Use for inputs that the server consumes once at Create
// and then discards (validation-only args with no server-side counterpart) —
// changing them in HCL later is meaningless and shouldn't generate a plan.
func suppressAfterCreate(_, _, _ string, d *schema.ResourceData) bool {
	return d.Id() != ""
}

func resourceComponentLink() *schema.Resource {
	return &schema.Resource{
		Description: "A link in a project's blueprint that connects one component's output field to another component's input field. " +
			"Links are design-time wiring on the canvas — when the linked components are deployed in an environment, the platform " +
			"automatically connects each producing instance's output to the corresponding consuming instance's input. " +
			"Links are versionless: a single `massdriver_component_link` applies to every deployed bundle version that still exposes " +
			"`from_field` and `to_field` under those names. If a future bundle version renames a field, declare a NEW " +
			"`massdriver_component_link` for the new name (leaving the old one in place for environments still on the older bundle) " +
			"rather than mutating this one.",

		CreateContext: resourceComponentLinkCreate,
		ReadContext:   resourceComponentLinkRead,
		UpdateContext: resourceComponentLinkUpdate,
		DeleteContext: resourceComponentLinkDelete,

		Schema: map[string]*schema.Schema{
			"from_component_id": {
				Description: "ID of the source (producer) component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"from_field": {
				Description: "Output field name on the source component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			// from_version and to_version are pure Create-time validation args:
			// the server uses them to confirm `from_field` / `to_field` exist
			// on the named bundle versions, then discards them. The persisted
			// link is versionless — it applies to every version of the linked
			// components that still expose those field names. If a future
			// bundle version renames a field, the migration is to declare a
			// NEW component_link with the new field names (leaving the old
			// one in place for older instances), not to mutate this one — so
			// these fields are NOT ForceNew, and changes to them in HCL after
			// Create are suppressed entirely. Field renames are caught by
			// ForceNew on `from_field` / `to_field`.
			"from_version": {
				Description: "Bundle version constraint used **only at Create time** to validate that `from_field` exists on the source bundle. " +
					"Once the API has validated the link, the version is discarded — the persisted link is versionless and applies " +
					"to every deployed instance whose bundle exposes `from_field` under that name. Defaults to `latest+dev`, which is the " +
					"right choice for almost every case; the only reason to set this explicitly is to validate against a historical bundle " +
					"version that uses a field name no longer present in `latest`. Changes to this attribute after Create are ignored (no diff, " +
					"no recreate).",
				Type:             schema.TypeString,
				Optional:         true,
				Default:          "latest+dev",
				DiffSuppressFunc: suppressAfterCreate,
			},
			"to_component_id": {
				Description: "ID of the destination (consumer) component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"to_field": {
				Description: "Input field name on the destination component.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"to_version": {
				Description: "Bundle version constraint used **only at Create time** to validate that `to_field` exists on the destination bundle. " +
					"Once the API has validated the link, the version is discarded — the persisted link is versionless and applies " +
					"to every deployed instance whose bundle exposes `to_field` under that name. Defaults to `latest+dev`, which is the " +
					"right choice for almost every case; the only reason to set this explicitly is to validate against a historical bundle " +
					"version that uses a field name no longer present in `latest`. Changes to this attribute after Create are ignored (no diff, " +
					"no recreate).",
				Type:             schema.TypeString,
				Optional:         true,
				Default:          "latest+dev",
				DiffSuppressFunc: suppressAfterCreate,
			},
			"project_id": {
				Description: "ID of the project this link lives in.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceComponentLinkCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	link, err := pc.ComponentLinks.AddLink(ctx, components.AddLinkInput{
		FromComponentID: d.Get("from_component_id").(string),
		FromField:       d.Get("from_field").(string),
		FromVersion:     d.Get("from_version").(string),
		ToComponentID:   d.Get("to_component_id").(string),
		ToField:         d.Get("to_field").(string),
		ToVersion:       d.Get("to_version").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(link.ID)

	// Persist project_id so subsequent Reads can locate this link via the
	// project's `links` field. The link's FromComponent carries its parent
	// project; if for any reason the SDK response didn't include the project
	// embed, fall back to fetching the from-component directly.
	projectID := ""
	if link.FromComponent != nil && link.FromComponent.Project != nil {
		projectID = link.FromComponent.Project.ID
	}
	if projectID == "" {
		comp, err := pc.Components.Get(ctx, d.Get("from_component_id").(string))
		if err != nil {
			return diag.FromErr(err)
		}
		if comp.Project == nil {
			return diag.Errorf("component %s has no parent project; cannot resolve project_id for the link", comp.ID)
		}
		projectID = comp.Project.ID
	}
	d.Set("project_id", projectID)
	return nil
}

// resourceComponentLinkRead confirms the link still exists by looking it up
// in the parent project's `links` list. We don't refresh any
// fields on Read: from_*/to_* are immutable (ForceNew), versions have no
// server-side counterpart, and project_id is set once at Create.
func resourceComponentLinkRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	projectID := d.Get("project_id").(string)
	if projectID == "" {
		// State predates project_id being a tracked attribute (or import,
		// when we add it later, hasn't populated it). Best we can do is
		// trust state and move on; the user can re-apply to refresh.
		return nil
	}

	project, err := pc.Projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			// Parent project is gone — the link is gone too.
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	linkID := d.Id()
	for _, link := range project.Links {
		if link.ID == linkID {
			return nil
		}
	}

	// Link not found in the project's link list — deleted out of band.
	d.SetId("")
	return nil
}

// resourceComponentLinkUpdate is a defensive no-op. Every actually-mutable
// field on this resource is ForceNew, and the version fields suppress diffs
// after Create — so terraform should never call Update in practice. It's
// declared only because the schema validator requires either Update OR
// ForceNew-on-every-Optional, and the latter is wrong for the version fields
// (see their DiffSuppressFunc rationale).
func resourceComponentLinkUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	return resourceComponentLinkRead(ctx, d, meta)
}

func resourceComponentLinkDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.ComponentLinks.RemoveLink(ctx, d.Id()); err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}
