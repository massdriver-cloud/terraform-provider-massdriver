package massdriver

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-provider-massdriver/internal/api"
)

func resourceInstanceAlarm() *schema.Resource {
	return &schema.Resource{
		Description: "Registers a cloud metric alarm with a Massdriver instance. State updates arrive via webhooks from CloudWatch / Azure Monitor / GCP Cloud Monitoring / Alertmanager. Replaces the v0 `massdriver_package_alarm`.",

		CreateContext: resourceInstanceAlarmCreate,
		ReadContext:   resourceInstanceAlarmRead,
		UpdateContext: resourceInstanceAlarmUpdate,
		DeleteContext: resourceInstanceAlarmDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"instance_id": {
				Description: "ID of the instance this alarm is attached to. Immutable after creation.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
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
	client := meta.(*ProviderClient).Client

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
	if v, ok := d.GetOk("period"); ok {
		p := v.(int)
		input.Period = &p
	}
	input.Metric = parseAlarmMetric(d.Get("metric").([]any))

	alarm, err := api.CreateInstanceAlarm(ctx, client, d.Get("instance_id").(string), input)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(alarm.ID)
	return resourceInstanceAlarmRead(ctx, d, meta)
}

func resourceInstanceAlarmRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	alarm, err := api.GetInstanceAlarm(ctx, client, d.Id())
	if err != nil {
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
	client := meta.(*ProviderClient).Client

	input := api.UpdateInstanceAlarmInput{
		CloudResourceId:    d.Get("cloud_resource_id").(string),
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

	if _, err := api.UpdateInstanceAlarm(ctx, client, d.Id(), input); err != nil {
		return diag.FromErr(err)
	}

	return resourceInstanceAlarmRead(ctx, d, meta)
}

func resourceInstanceAlarmDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client := meta.(*ProviderClient).Client

	if _, err := api.DeleteInstanceAlarm(ctx, client, d.Id()); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

// parseAlarmMetric converts the optional metric block from terraform's nested-list
// representation into the API input. Returns nil when the block is omitted, which
// makes the field disappear from the JSON body via `omitempty`.
func parseAlarmMetric(block []any) *api.AlarmMetricInput {
	if len(block) == 0 || block[0] == nil {
		return nil
	}
	raw, ok := block[0].(map[string]any)
	if !ok {
		return nil
	}
	metric := &api.AlarmMetricInput{
		Namespace: stringFrom(raw, "namespace"),
		Name:      stringFrom(raw, "name"),
		Statistic: stringFrom(raw, "statistic"),
		Region:    stringFrom(raw, "region"),
	}
	if dims, ok := raw["dimensions"].(map[string]any); ok {
		for k, v := range dims {
			s, _ := v.(string)
			metric.Dimensions = append(metric.Dimensions, api.AlarmMetricDimensionInput{
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

func dimensionsToMap(dims []api.AlarmMetricDimension) map[string]string {
	out := make(map[string]string, len(dims))
	for _, d := range dims {
		out[d.Name] = d.Value
	}
	return out
}
