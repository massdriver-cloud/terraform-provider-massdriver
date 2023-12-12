package massdriver

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type PackageAlarmMetric struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Statistic  string            `json:"statistic,omitempty"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

type PackageAlarmMetadata struct {
	ResourceIdentifier string              `json:"cloud_resource_id"`
	DisplayName        string              `json:"display_name"`
	Metric             *PackageAlarmMetric `json:"metric,omitempty"`
	Threshold          float64             `json:"threshold,omitempty"`
	PeriodMinutes      int                 `json:"period_minutes,omitempty"`
	ComparisonOperator string              `json:"comparsion_operator,omitempty"`
}

func resourcePackageAlarm() *schema.Resource {
	return &schema.Resource{
		Description: "This resource registers a package alarm in the Massdriver console for presentation to the user",

		CreateContext: resourcePackageAlarmCreate,
		ReadContext:   schema.NoopContext,
		DeleteContext: resourcePackageAlarmDelete,

		Schema: map[string]*schema.Schema{
			"cloud_resource_id": {
				Description: "The identifier of the alarm. In Azure it will be the id, GCP will be the name, and in AWS it will be the arn",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"display_name": {
				Description: "The name to display in the massdriver UI",
				Type:        schema.TypeString,
				ForceNew:    true,
				Required:    true,
			},
			"metric": {
				Type:     schema.TypeList,
				MaxItems: 1,
				ForceNew: true,
				Optional: true, // This should be removed when we've added it to all our existing alarms
				//Required: false,     This should be set to true when we've added it to all our existing alarms
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Description: "Name of the metric. Required for all clouds.",
							ForceNew:    true,
							Required:    true,
						},
						"namespace": {
							Type:        schema.TypeString,
							Description: "Namespace of the metric. Required for AWS and Azure. Omit for GCP.",
							ForceNew:    true,
							Required:    true,
						},
						"statistic": {
							Type:        schema.TypeString,
							Description: "Aggregation method (sum, average, maximum, etc.)",
							ForceNew:    true,
							Optional:    true,
						},
						"dimensions": {
							Type:        schema.TypeMap,
							Description: "The filtering criteria for the metric",
							ForceNew:    true,
							Optional:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"threshold": {
				Description: "The threshold for triggerin the alarm",
				Type:        schema.TypeFloat,
				ForceNew:    true,
				Optional:    true,
			},
			"period_minutes": {
				Description: "The number of periods over which data is compared to the specified threshold",
				Type:        schema.TypeInt,
				ForceNew:    true,
				Optional:    true,
			},
			"comparison_operator": {
				Description: "The operation to use when comparing the specified statistic and threshold",
				Type:        schema.TypeString,
				ForceNew:    true,
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

func resourcePackageAlarmCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	packageAlarmMeta := PackageAlarmMetadata{
		ResourceIdentifier: d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
		Metric:             parseMetricBock(d.Get("metric").([]interface{})),
		Threshold:          d.Get("threshold").(float64),
		PeriodMinutes:      d.Get("period_minutes").(int),
		ComparisonOperator: d.Get("comparison_operator").(string),
	}

	event := NewEvent(EVENT_TYPE_ALARM_CHANNEL_CREATED)
	event.Payload = EventPayloadAlarmChannels{DeploymentId: c.DeploymentID, PackageAlarm: packageAlarmMeta}

	err := c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId(time.Now().Format(time.RFC3339))
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*MassdriverClient)

	var diags diag.Diagnostics

	packageAlarmMeta := PackageAlarmMetadata{
		ResourceIdentifier: d.Get("cloud_resource_id").(string),
		DisplayName:        d.Get("display_name").(string),
		Metric:             parseMetricBock(d.Get("metric").([]interface{})),
		Threshold:          d.Get("threshold").(float64),
		PeriodMinutes:      d.Get("period_minutes").(int),
		ComparisonOperator: d.Get("comparison_operator").(string),
	}

	event := NewEvent(EVENT_TYPE_ALARM_CHANNEL_DELETED)
	event.Payload = EventPayloadAlarmChannels{DeploymentId: c.DeploymentID, PackageAlarm: packageAlarmMeta}

	err := c.PublishEventToSNS(event, &diags)

	if err != nil {
		return diags
	}

	d.SetId("")

	return diags
}

func parseMetricBock(block []interface{}) *PackageAlarmMetric {
	if len(block) == 0 {
		return nil
	}
	metric := new(PackageAlarmMetric)

	blockMap := block[0].(map[string]interface{})

	metric.Name = blockMap["name"].(string)
	metric.Statistic = blockMap["statistic"].(string)

	if namespace, ok := blockMap["namespace"]; ok {
		metric.Namespace = namespace.(string)
	}
	if dimensions, ok := blockMap["dimensions"]; ok {
		metric.Dimensions = make(map[string]string, len(metric.Dimensions))
		for key, value := range dimensions.(map[string]interface{}) {
			metric.Dimensions[key] = value.(string)
		}
	}

	return metric
}
