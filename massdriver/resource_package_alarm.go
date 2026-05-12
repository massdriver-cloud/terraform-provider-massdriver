package massdriver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"terraform-provider-massdriver/internal/api"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourcePackageAlarm() *schema.Resource {
	return &schema.Resource{
		Description:        "This resource registers a package alarm in the Massdriver console for presentation to the user.",
		DeprecationMessage: "massdriver_package_alarm is deprecated and will be removed in v2.0 of the massdriver provider. Use `massdriver_instance_alarm` instead. Do not manage the same alarm via both `massdriver_package_alarm` and `massdriver_instance_alarm` — terraform will not detect the conflict and the two resources will fight over state.",

		CreateContext: resourcePackageAlarmCreate,
		ReadContext:   resourcePackageAlarmRead,
		UpdateContext: resourcePackageAlarmUpdate,
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
				Description: "The package ID associated with this alarm. This should generally be left unspecified, since the package ID will be read from the MASSDRIVER_PACKAGE_NAME environment variable.",
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
}

// resourcePackageAlarmCreate first looks up an existing alarm on the instance
// matching cloud_resource_id (self-heal). If found, we adopt it into state
// rather than creating a duplicate — this recovers from pre-1.3 deploys where
// the old REST-based Read 404'd against the now-dead endpoint and silently
// cleared the alarm's UUID from state, leaving the server-side record
// orphaned. If no existing alarm matches, we create a new one via GraphQL.
//
// Lookup errors are surfaced verbatim so a transient API/auth/network issue
// is not misreported as a missing alarm.
func resourcePackageAlarmCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	instanceID, err := getPackageShortName(d)
	if err != nil {
		return diag.FromErr(err)
	}
	cloudResourceID := d.Get("cloud_resource_id").(string)

	client := meta.(*ProviderClient).Client
	existing, err := api.FindInstanceAlarmByCloudResourceID(ctx, client, instanceID, cloudResourceID)
	if err != nil {
		return diag.FromErr(err)
	}
	if existing != nil {
		d.SetId(existing.ID)
		return resourcePackageAlarmRead(ctx, d, meta)
	}

	alarm, err := api.CreateInstanceAlarm(ctx, client, instanceID, buildCreateInstanceAlarmInput(d))
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(alarm.ID)
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return resourcePackageAlarmRead(ctx, d, meta)
}

func resourcePackageAlarmUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client
	if _, err := api.UpdateInstanceAlarm(ctx, client, d.Id(), buildUpdateInstanceAlarmInput(d)); err != nil {
		return diag.FromErr(err)
	}
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return resourcePackageAlarmRead(ctx, d, meta)
}

// getPackageShortName resolves the instance identifier used by the GraphQL
// API. The package_alarm HCL field is `package_id` and bundle deployments
// inject MASSDRIVER_PACKAGE_NAME — both carry the package's deployment-suffixed
// name (e.g., `bundtst-plygrnd-awsaurorapos-rbpt`). The instance lookup needs
// the short form (`bundtst-plygrnd-awsaurorapos`), so we strip the last
// hyphen-segment.
//
// `package_id` from HCL wins over the env var; the env var is only a fallback
// for older configs that didn't surface the field explicitly.
func getPackageShortName(d *schema.ResourceData) (string, error) {
	var fullName string
	if pkg, ok := d.Get("package_id").(string); ok && pkg != "" {
		fullName = pkg
	} else if pkg := os.Getenv("MASSDRIVER_PACKAGE_NAME"); pkg != "" {
		fullName = pkg
	}
	if fullName == "" {
		return "", fmt.Errorf("`package_id` must be set in config or MASSDRIVER_PACKAGE_NAME must be set in the environment")
	}
	parts := strings.Split(fullName, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("`package_id` %q must contain at least one hyphen", fullName)
	}
	return strings.Join(parts[:len(parts)-1], "-"), nil
}

// resourcePackageAlarmRead hydrates state from the GraphQL instance_alarm
// endpoint. The package_alarm REST endpoint has been removed from the server;
// the underlying alarm data (and IDs) carry over into the instance_alarm
// schema unchanged, so we read by the same ID.
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
		// Out-of-band deletion: clear state so terraform plans a recreate.
		if strings.Contains(err.Error(), "not found") {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("cloud_resource_id", alarm.CloudResourceID)
	d.Set("display_name", alarm.DisplayName)
	d.Set("threshold", alarm.Threshold)
	// instance_alarm exposes period in seconds; package_alarm tracks minutes.
	// Integer division rounds non-multiple-of-60 periods down, which surfaces
	// as drift on the next plan — acceptable since the user can switch to
	// `massdriver_instance_alarm` (which uses seconds) to fix it.
	d.Set("period_minutes", alarm.Period/60)
	d.Set("comparison_operator", alarm.ComparisonOperator)

	if alarm.Metric == nil {
		// Explicitly clear so a server-side metric removal surfaces as drift
		// instead of state retaining a stale block forever.
		if err := d.Set("metric", []any{}); err != nil {
			return diag.FromErr(err)
		}
	} else {
		dimensions := make(map[string]any, len(alarm.Metric.Dimensions))
		for _, dim := range alarm.Metric.Dimensions {
			dimensions[dim.Name] = dim.Value
		}
		// Region is intentionally not surfaced — the v1 schema doesn't expose
		// it, so reading it would create unmappable drift.
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

func buildCreateInstanceAlarmInput(d *schema.ResourceData) api.CreateInstanceAlarmInput {
	input := api.CreateInstanceAlarmInput{
		CloudResourceId: d.Get("cloud_resource_id").(string),
		DisplayName:     d.Get("display_name").(string),
	}
	if v, ok := d.GetOk("comparison_operator"); ok {
		input.ComparisonOperator = v.(string)
	}
	if v, ok := d.GetOk("threshold"); ok {
		f := v.(float64)
		input.Threshold = &f
	}
	if v, ok := d.GetOk("period_minutes"); ok {
		p := v.(int) * 60
		input.Period = &p
	}
	input.Metric = parseAlarmMetric(d.Get("metric").([]any))
	return input
}

func buildUpdateInstanceAlarmInput(d *schema.ResourceData) api.UpdateInstanceAlarmInput {
	input := api.UpdateInstanceAlarmInput{
		CloudResourceId:    d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
		ComparisonOperator: d.Get("comparison_operator").(string),
	}
	if v, ok := d.GetOk("threshold"); ok {
		f := v.(float64)
		input.Threshold = &f
	}
	if v, ok := d.GetOk("period_minutes"); ok {
		p := v.(int) * 60
		input.Period = &p
	}
	input.Metric = parseAlarmMetric(d.Get("metric").([]any))
	return input
}

// parseAlarmMetric and stringFrom are defined in resource_instance_alarm.go
// and shared with package_alarm. The shared parser also populates `Region`,
// which the v1 package_alarm schema doesn't expose — empty Region is fine
// because the GraphQL input directive drops empty values from the wire.
