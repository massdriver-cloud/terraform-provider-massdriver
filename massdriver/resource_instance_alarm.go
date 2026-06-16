package massdriver

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/instances"
)

// instanceAlarmsAPI is the slice of *instances.Service that this resource
// calls. Lives next to the resource so the interface stays tight to the
// methods actually used; *instances.Service satisfies it in production,
// hand-rolled fakes satisfy it in tests.
type instanceAlarmsAPI interface {
	GetAlarm(ctx context.Context, id string) (*instances.Alarm, error)
	CreateAlarm(ctx context.Context, instanceID string, input instances.CreateAlarmInput) (*instances.Alarm, error)
	UpdateAlarm(ctx context.Context, id string, input instances.UpdateAlarmInput) (*instances.Alarm, error)
	DeleteAlarm(ctx context.Context, id string) (*instances.Alarm, error)
}

func resourceInstanceAlarm() *schema.Resource {
	return &schema.Resource{
		Description: "Registers a cloud metric alarm with a Massdriver instance. State updates arrive via webhooks from CloudWatch / Azure Monitor / GCP Cloud Monitoring / Alertmanager. Replaces the v1 `massdriver_package_alarm`.",

		CreateContext: resourceInstanceAlarmCreate,
		ReadContext:   resourceInstanceAlarmRead,
		UpdateContext: resourceInstanceAlarmUpdate,
		DeleteContext: resourceInstanceAlarmDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"instance_id": {
				Description: "ID of the instance this alarm is attached to. Defaults to `MASSDRIVER_INSTANCE_ID` if set, otherwise to `MASSDRIVER_PACKAGE_NAME` with the trailing deployment suffix stripped (so bundles get the right ID for free). Must be set explicitly when running outside a Massdriver deployment. Immutable after creation.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				DefaultFunc: instanceIDFromEnv,
			},
			"display_name": {
				Description: "Human-readable name shown in the Massdriver UI and notifications.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"cloud_resource_id": {
				Description: "Cloud provider's unique identifier for the alarm (e.g., a CloudWatch AlarmArn). Used to correlate inbound webhooks back to this alarm. Must be unique within the instance.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"comparison_operator": {
				Description: "How the metric is compared against `threshold` (e.g., `GREATER_THAN`, `LESS_THAN`). May be empty for providers that don't expose this concept (Alertmanager, GCP).",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"threshold": {
				Description: "Value crossed to trigger the alarm.",
				Type:        schema.TypeFloat,
				Optional:    true,
			},
			"period": {
				Description: "Evaluation window in seconds over which the metric is aggregated.",
				Type:        schema.TypeInt,
				Optional:    true,
			},
			"metric": {
				Description: "Cloud metric the alarm evaluates. Optional — providers like Alertmanager don't supply structured metric data.",
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"namespace": {
							Description: "Cloud service namespace (e.g., `AWS/RDS`).",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"name": {
							Description: "Metric name within the namespace (e.g., `CPUUtilization`).",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"statistic": {
							Description: "Aggregation function (e.g., `Average`). Empty for providers without it.",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"region": {
							Description: "Cloud region the metric is scoped to, when applicable.",
							Type:        schema.TypeString,
							Optional:    true,
						},
						"dimensions": {
							Description: "Key-value dimensions identifying the monitored resource. Empty when the provider doesn't expose structured dimensions.",
							Type:        schema.TypeMap,
							Optional:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
		},
	}
}

func resourceInstanceAlarmCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	instanceID := d.Get("instance_id").(string)
	if instanceID == "" {
		return diag.Errorf("instance_id must be set in config, or MASSDRIVER_INSTANCE_ID / MASSDRIVER_PACKAGE_NAME must be set in the environment")
	}

	input := instances.CreateAlarmInput{
		CloudResourceID: d.Get("cloud_resource_id").(string),
		DisplayName:     d.Get("display_name").(string),
	}
	if v, ok := d.GetOk("comparison_operator"); ok {
		input.ComparisonOperator = v.(string)
	}
	if v, ok := d.GetOk("threshold"); ok {
		f := v.(float64)
		input.Threshold = &f
	}
	if v, ok := d.GetOk("period"); ok {
		p := v.(int)
		input.Period = &p
	}
	input.Metric = parseAlarmMetric(d.Get("metric").([]any))

	alarm, err := pc.InstanceAlarms.CreateAlarm(ctx, instanceID, input)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(alarm.ID)
	return resourceInstanceAlarmRead(ctx, d, meta)
}

func resourceInstanceAlarmRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	alarm, err := pc.InstanceAlarms.GetAlarm(ctx, d.Id())
	if err != nil {
		// Out-of-band deletion: clear state so terraform plans a recreate.
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.Set("display_name", alarm.DisplayName)
	d.Set("cloud_resource_id", alarm.CloudResourceID)
	d.Set("comparison_operator", alarm.ComparisonOperator)
	d.Set("threshold", alarm.Threshold)
	d.Set("period", alarm.Period)

	if alarm.Metric == nil {
		d.Set("metric", nil)
	} else {
		metric := map[string]any{
			"namespace":  alarm.Metric.Namespace,
			"name":       alarm.Metric.Name,
			"statistic":  alarm.Metric.Statistic,
			"region":     alarm.Metric.Region,
			"dimensions": dimensionsToMap(alarm.Metric.Dimensions),
		}
		if err := d.Set("metric", []any{metric}); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceInstanceAlarmUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	input := instances.UpdateAlarmInput{
		CloudResourceID:    d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
		ComparisonOperator: d.Get("comparison_operator").(string),
	}
	if v, ok := d.GetOk("threshold"); ok {
		f := v.(float64)
		input.Threshold = &f
	}
	if v, ok := d.GetOk("period"); ok {
		p := v.(int)
		input.Period = &p
	}
	input.Metric = parseAlarmMetric(d.Get("metric").([]any))

	if _, err := pc.InstanceAlarms.UpdateAlarm(ctx, d.Id(), input); err != nil {
		return diag.FromErr(err)
	}

	return resourceInstanceAlarmRead(ctx, d, meta)
}

func resourceInstanceAlarmDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.InstanceAlarms.DeleteAlarm(ctx, d.Id()); err != nil {
		// "Already gone" doesn't block destroy.
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// instanceIDFromEnv is the DefaultFunc for `instance_id`. We read env vars
// directly rather than going through provisioning.Config: that Config errors
// when *any* deployment env var is missing (an intentional design of the
// provisioning surface), but the DefaultFunc must succeed even outside a
// bundle deployment — its job is to provide a default if the env exists, and
// otherwise leave the field empty so the user can supply it via HCL.
//
// MASSDRIVER_INSTANCE_ID wins when set (canonical, matches the SDK's
// provisioning.Config.InstanceID field). If only the legacy
// MASSDRIVER_PACKAGE_NAME is present (older bundle deploys), strip the
// trailing deployment suffix (e.g. `bundtst-plygrnd-awsaurorapos-rbpt` →
// `bundtst-plygrnd-awsaurorapos`).
func instanceIDFromEnv() (any, error) {
	if id := os.Getenv("MASSDRIVER_INSTANCE_ID"); id != "" {
		return id, nil
	}
	name := os.Getenv("MASSDRIVER_PACKAGE_NAME")
	if name == "" {
		return nil, nil
	}
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return name, nil
	}
	return strings.Join(parts[:len(parts)-1], "-"), nil
}

// parseAlarmMetric converts the optional `metric` HCL block into the SDK
// input. Returns nil when the block is omitted so the SDK leaves the field
// unset on the wire.
func parseAlarmMetric(block []any) *instances.AlarmMetric {
	if len(block) == 0 || block[0] == nil {
		return nil
	}
	raw, ok := block[0].(map[string]any)
	if !ok {
		return nil
	}
	metric := &instances.AlarmMetric{
		Namespace: stringFrom(raw, "namespace"),
		Name:      stringFrom(raw, "name"),
		Statistic: stringFrom(raw, "statistic"),
		Region:    stringFrom(raw, "region"),
	}
	if dims, ok := raw["dimensions"].(map[string]any); ok {
		for k, v := range dims {
			s, _ := v.(string)
			metric.Dimensions = append(metric.Dimensions, instances.AlarmMetricDimension{
				Name:  k,
				Value: s,
			})
		}
	}
	return metric
}

func stringFrom(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func dimensionsToMap(dims []instances.AlarmMetricDimension) map[string]string {
	out := make(map[string]string, len(dims))
	for _, d := range dims {
		out[d.Name] = d.Value
	}
	return out
}
