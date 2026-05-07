package massdriver

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

func resourcePackageAlarm() *schema.Resource {
	r := &schema.Resource{
		Description:        "This resource registers a package alarm in the Massdriver console for presentation to the user",
		DeprecationMessage: "massdriver_package_alarm is deprecated. Create and update operations are no longer supported — use massdriver_instance_alarm instead. Existing massdriver_package_alarm resources can still be refreshed and destroyed.",

		CreateContext: resourcePackageAlarmWritesDisabled,
		ReadContext:   resourcePackageAlarmRead,
		UpdateContext: resourcePackageAlarmWritesDisabled,
		DeleteContext: resourcePackageAlarmDelete,

		Schema: map[string]*schema.Schema{
			"cloud_resource_id": {
				Description: "The identifier of the alarm. In Azure it will be the id, GCP will be the name, and in AWS it will be the arn",
				Type:        schema.TypeString,
				Required:    true,
			},
			"display_name": {
				Description: "The name to display in the massdriver UI",
				Type:        schema.TypeString,
				Required:    true,
			},
			"metric": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Description: "Name of the metric. Required for all clouds.",
							Required:    true,
						},
						"namespace": {
							Type:        schema.TypeString,
							Description: "Namespace of the metric. Required for AWS and Azure. Omit for GCP.",
							Required:    true,
						},
						"statistic": {
							Type:        schema.TypeString,
							Description: "Aggregation method (sum, average, maximum, etc.)",
							Optional:    true,
						},
						"dimensions": {
							Type:        schema.TypeMap,
							Description: "The filtering criteria for the metric",
							Optional:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"package_id": {
				Description: "The package ID associated with this alarm. Retained for backward compatibility with existing state; not refreshed from the server.",
				Type:        schema.TypeString,
				ForceNew:    true,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("MASSDRIVER_PACKAGE_NAME", nil),
			},
			"threshold": {
				Description: "The threshold for triggerin the alarm",
				Type:        schema.TypeFloat,
				Optional:    true,
			},
			"period_minutes": {
				Description: "The number of periods over which data is compared to the specified threshold",
				Type:        schema.TypeInt,
				Optional:    true,
			},
			"comparison_operator": {
				Description: "The operation to use when comparing the specified statistic and threshold",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"last_updated": {
				Description: "A timestamp of when the last time this resource was updated",
				Type:        schema.TypeString,
				Optional:    false,
				Required:    false,
				Computed:    true,
			},
		},
	}
	// Decorate every attribute with a DiffSuppressFunc so terraform never
	// plans an Update — see suppressAllDiffs for why.
	suppressAllDiffs(r.Schema)
	return r
}

// suppressAllDiffs decorates every attribute in the given schema map with a
// DiffSuppressFunc that returns true, recursing into nested `Resource`
// elements. Wired into massdriver_package_alarm so terraform never plans an
// Update — the underlying Create/Update API endpoints have been removed and
// an Update call would just hard-error via resourcePackageAlarmWritesDisabled.
//
// The point is to keep existing terraform configurations applying cleanly
// while users transition to massdriver_instance_alarm at their own pace.
// Without this, any drift between HCL and state — either from a server-side
// change picked up by Read, or just stale config — would cause `terraform
// apply` to fail.
//
// `CustomizeDiff + d.Clear(key)` would be the more obvious approach but
// `Clear` only works on Computed attributes, and most package_alarm fields
// are Required/Optional. Per-attribute DiffSuppressFunc has no such
// restriction.
//
// Destroy is unaffected (terraform's destroy phase doesn't go through the
// resource's diff machinery), so users can still `terraform destroy` to
// remove these resources from state.
func suppressAllDiffs(schemaMap map[string]*schema.Schema) {
	for _, s := range schemaMap {
		suppressFieldDiff(s)
	}
}

// suppressFieldDiff is the per-attribute walker that suppressAllDiffs
// dispatches to. For nested `Resource` element schemas (e.g., the `metric`
// block) it recurses into the children so attributes like
// `metric.0.namespace` also get suppressed.
//
// Computed-only fields are skipped: the SDK rejects DiffSuppressFunc on them
// at InternalValidate time ("no config to compare"), and they can't drive an
// Update anyway since the user has no way to set them in HCL.
func suppressFieldDiff(s *schema.Schema) {
	if s == nil {
		return
	}
	if !(s.Computed && !s.Optional && !s.Required) {
		s.DiffSuppressFunc = func(_, _, _ string, _ *schema.ResourceData) bool {
			return true
		}
	}
	if elem, ok := s.Elem.(*schema.Resource); ok {
		for _, sub := range elem.Schema {
			suppressFieldDiff(sub)
		}
	}
}

// resourcePackageAlarmWritesDisabled is the wired-in CreateContext /
// UpdateContext for the deprecated massdriver_package_alarm resource. It
// returns a hard error so users can't introduce new package alarms or mutate
// existing ones — they must migrate to massdriver_instance_alarm. Read and
// Delete are intentionally still functional so existing state remains usable
// while users transition off.
func resourcePackageAlarmWritesDisabled(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics {
	return diag.Errorf("massdriver_package_alarm no longer supports create or update operations. Use massdriver_instance_alarm instead. Existing massdriver_package_alarm resources in state can still be refreshed and destroyed.")
}

// resourcePackageAlarmRead reads via the instance_alarm GraphQL endpoint.
// The package_alarm REST endpoint has been removed from the server — the
// underlying alarm data (and its IDs) carry over into the instance_alarm
// schema unchanged, so we can read it via GraphQL by the same ID.
func resourcePackageAlarmRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	// Pre-modern (timestamp-format) IDs date back before the server assigned
	// real UUIDs. Those won't parse as UUID and the GraphQL endpoint will
	// reject them — leave state untouched so users can `terraform state rm`
	// or destroy without a refresh failure.
	if isLegacyTimestampID(d.Id()) {
		return nil
	}

	client := meta.(*ProviderClient).Client
	alarm, err := api.GetInstanceAlarm(ctx, client, d.Id())
	if err != nil {
		// Mirror the legacy behavior: surface a "not found" as state-clear so
		// terraform plans a recreate (which then hard-errors via WritesDisabled
		// and forces the user to migrate to massdriver_instance_alarm).
		if strings.Contains(err.Error(), "not found") {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("cloud_resource_id", alarm.CloudResourceID)
	d.Set("display_name", alarm.DisplayName)

	if alarm.Threshold != 0 {
		d.Set("threshold", alarm.Threshold)
	}
	// instance_alarm exposes period in seconds; package_alarm tracked it in
	// minutes. Convert with integer division — non-multiple-of-60 periods
	// (rare in practice) round down, but the user can't update the field via
	// terraform anyway so any drift is informational only.
	if alarm.Period != 0 {
		d.Set("period_minutes", alarm.Period/60)
	}
	if alarm.ComparisonOperator != "" {
		d.Set("comparison_operator", alarm.ComparisonOperator)
	}

	if alarm.Metric != nil {
		dimensions := make(map[string]any, len(alarm.Metric.Dimensions))
		for _, dim := range alarm.Metric.Dimensions {
			dimensions[dim.Name] = dim.Value
		}
		metric := map[string]any{
			"name":       alarm.Metric.Name,
			"namespace":  alarm.Metric.Namespace,
			"statistic":  alarm.Metric.Statistic,
			"dimensions": dimensions,
		}
		if err := d.Set("metric", []any{metric}); err != nil {
			return diag.FromErr(err)
		}
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))
	return nil
}

// resourcePackageAlarmDelete deletes via the instance_alarm GraphQL endpoint.
// Legacy timestamp-format IDs are simply dropped from state — the REST
// endpoint that knew how to delete them is gone, and the underlying server-
// side records (if any) will be cleaned up by a server migration.
func resourcePackageAlarmDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	id := d.Id()
	if id == "" {
		return nil
	}
	if isLegacyTimestampID(id) {
		d.SetId("")
		return nil
	}

	client := meta.(*ProviderClient).Client
	if _, err := api.DeleteInstanceAlarm(ctx, client, id); err != nil {
		// "not found" means it's already gone server-side; clear state and
		// move on rather than blocking a destroy on a phantom resource.
		if strings.Contains(err.Error(), "not found") {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// isLegacyTimestampID returns true for IDs from the pre-UUID era of the REST
// API where the resource ID was the RFC3339 timestamp at create time. The
// GraphQL endpoint rejects these as UUID parse errors; we sidestep the call.
func isLegacyTimestampID(id string) bool {
	_, err := time.Parse(time.RFC3339, id)
	return err == nil
}
