package massdriver

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/services/packagealarms"
)

func resourcePackageAlarm() *schema.Resource {
	return &schema.Resource{
		Description: "This resource registers a package alarm in the Massdriver console for presentation to the user",

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
				Optional: true, // This should be removed when we've added it to all our existing alarms
				//Required: false,     This should be set to true when we've added it to all our existing alarms
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

func resourcePackageAlarmCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	service := meta.(*ProviderClient).PackageAlarmsService()

	var diags diag.Diagnostics

	alarm := parseAlarmBlock(d)

	packageID, idErr := getPackageName(d)
	if idErr != nil {
		return diag.FromErr(idErr)
	}

	resp, createErr := service.CreatePackageAlarm(ctx, packageID, alarm)
	if createErr != nil {
		return diag.FromErr(createErr)
	}

	d.SetId(resp.ID)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	service := meta.(*ProviderClient).PackageAlarmsService()

	var diags diag.Diagnostics

	// If the ID is a timestamp, it was from the older system where we didn't have IDs. The package_id field should force a new creation. Don't lookup, just bail.
	if _, err := time.Parse(time.RFC3339, d.Id()); err == nil {
		return nil
	}

	packageID, idErr := getPackageName(d)
	if idErr != nil {
		return diag.FromErr(idErr)
	}

	alarm, getErr := service.GetPackageAlarm(ctx, packageID, d.Id())
	if getErr != nil {
		if getErr.Error() == "not found" {
			d.SetId("")
			return diags
		}
		return diag.FromErr(getErr)
	}

	d.Set("cloud_resource_id", alarm.CloudResourceID)
	d.Set("display_name", alarm.DisplayName)

	if alarm.Threshold != 0 {
		d.Set("threshold", alarm.Threshold)
	}
	if alarm.PeriodMinutes != 0 {
		d.Set("period_minutes", alarm.PeriodMinutes)
	}
	if alarm.ComparisonOperator != "" {
		d.Set("comparison_operator", alarm.ComparisonOperator)
	}

	if alarm.Metric != nil {
		metric := map[string]interface{}{
			"name":       alarm.Metric.Name,
			"namespace":  alarm.Metric.Namespace,
			"statistic":  alarm.Metric.Statistic,
			"dimensions": map[string]interface{}{},
		}

		if alarm.Metric.Dimensions != nil {
			for _, dimension := range alarm.Metric.Dimensions {
				metric["dimensions"].(map[string]interface{})[dimension.Name] = dimension.Value
			}
		}

		if err := d.Set("metric", []interface{}{metric}); err != nil {
			return diag.FromErr(err)
		}
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	service := meta.(*ProviderClient).PackageAlarmsService()

	var diags diag.Diagnostics

	alarm := parseAlarmBlock(d)

	packageID, idErr := getPackageName(d)
	if idErr != nil {
		return diag.FromErr(idErr)
	}

	_, updateErr := service.UpdatePackageAlarm(ctx, packageID, d.Id(), alarm)
	if updateErr != nil {
		return diag.FromErr(updateErr)
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourcePackageAlarmDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	service := meta.(*ProviderClient).PackageAlarmsService()

	var diags diag.Diagnostics

	alarmID := d.Id()
	if alarmID == "" {
		return diags
	}

	// If the ID is a timestamp, it was from the older system where we didn't have IDs. We need to delete using the cloud resource ID base64 encoded (with no padding)
	if _, err := time.Parse(time.RFC3339, alarmID); err == nil {
		alarmID = base64.RawURLEncoding.EncodeToString([]byte(d.Get("cloud_resource_id").(string)))
	}

	packageID, idErr := getPackageName(d)
	if idErr != nil {
		return diag.FromErr(idErr)
	}

	deleteErr := service.DeletePackageAlarm(ctx, packageID, alarmID)
	if deleteErr != nil {
		return diag.FromErr(deleteErr)
	}

	d.SetId("")

	return diags
}

func getPackageName(d *schema.ResourceData) (string, error) {
	packageID, ok := d.Get("package_id").(string)
	if !ok || packageID == "" {
		packageID = os.Getenv("MASSDRIVER_PACKAGE_NAME")
		if packageID == "" {
			return "", fmt.Errorf("`package_id` must be set in config or MASSDRIVER_PACKAGE_NAME must be set in the environment")
		}
	}
	return packageID, nil
}

func parseAlarmBlock(d *schema.ResourceData) *packagealarms.Alarm {
	alarm := new(packagealarms.Alarm)

	alarm.CloudResourceID = d.Get("cloud_resource_id").(string)
	alarm.DisplayName = d.Get("display_name").(string)
	alarm.Threshold = d.Get("threshold").(float64)
	alarm.PeriodMinutes = d.Get("period_minutes").(int)
	alarm.ComparisonOperator = d.Get("comparison_operator").(string)
	alarm.Metric = parseMetricBock(d.Get("metric").([]interface{}))

	return alarm
}

func parseMetricBock(block []interface{}) *packagealarms.Metric {
	if len(block) == 0 {
		return nil
	}
	metric := new(packagealarms.Metric)

	blockMap := block[0].(map[string]interface{})

	metric.Name = blockMap["name"].(string)
	metric.Statistic = blockMap["statistic"].(string)

	if namespace, ok := blockMap["namespace"]; ok {
		metric.Namespace = namespace.(string)
	}
	if dimensions, ok := blockMap["dimensions"]; ok {
		for key, value := range dimensions.(map[string]interface{}) {
			metric.Dimensions = append(metric.Dimensions, packagealarms.Dimension{
				Name:  key,
				Value: value.(string),
			})
		}
	}

	return metric
}
