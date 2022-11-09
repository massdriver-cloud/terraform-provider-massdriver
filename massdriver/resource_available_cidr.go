package massdriver

import (
	"context"
	"net"

	"github.com/massdriver-cloud/cola/pkg/cidr"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceAvailableCidr() *schema.Resource {
	return &schema.Resource{
		Description: "A Massdriver artifact for exporting a connectable type",

		CreateContext: resourceAvailableCidrCreate,
		ReadContext:   schema.NoopContext,
		UpdateContext: schema.NoopContext, //resourceArtifactUpdate,
		DeleteContext: resourceAvailableCidrDelete,

		Schema: map[string]*schema.Schema{
			"parent_cidrs": {
				Description: "A list of the CIDR range(s) from which to search for available CIDR ranges. Changing this value after creation **HAS NO EFFECT**. This allows the `result` CIDR to remain stable when it is used to find a range to create a network/subnet. If you would like to conditionally update this resource, use the `keepers` field.",
				Type:        schema.TypeList,
				MinItems:    1,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.IsCIDR,
				},
				Required: true,
			},
			"used_cidrs": {
				Description: "CIDR ranges that are already used within the `parent_cidrs` which should be avoided to prevent overlaps and/or collisions. Changing this value after creation **HAS NO EFFECT**. This allows the `result` CIDR to remain stable when it is used to find a range to create a network/subnet. If you would like to conditionally update this resource, use the `keepers` field.",
				Type:        schema.TypeList,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validation.IsCIDR,
				},
				Required: true,
			},
			"mask": {
				Description: "Desired mask (network/subnet size) to find that is available. Changing this value after creation **HAS NO EFFECT**. This allows the `result` CIDR to remain stable when it is used to find a range to create a network/subnet. If you would like to conditionally update this resource, use the `keepers` field.",
				Type:        schema.TypeInt,
				Required:    true,
			},
			"keepers": {
				Description: "Arbitrary map of values that, when changed, will trigger recreation of the resource. See [the main provider documentation](../index.html) for more information.",
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
			},
			"result": {
				Description: "A human readable name for this artifact.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceAvailableCidrCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	parentCidrsAtt := d.Get("parent_cidrs").([]interface{})
	usedCidrsAtt := d.Get("used_cidrs").([]interface{})
	maskAtt := d.Get("mask").(int)

	mask := net.CIDRMask(maskAtt, 32)

	usedCidrs := make([]*net.IPNet, len(usedCidrsAtt))
	for i, u := range usedCidrsAtt {
		_, usedCIDR, parseErr := net.ParseCIDR(u.(string))
		if parseErr != nil {
			diags = append(diags, diag.FromErr(parseErr)...)
			return diags
		}
		usedCidrs[i] = usedCIDR
	}

	var result *net.IPNet
	for _, p := range parentCidrsAtt {
		_, parentCidr, parseErr := net.ParseCIDR(p.(string))
		if parseErr != nil {
			diags = append(diags, diag.FromErr(parseErr)...)
			return diags
		}

		var findErr error
		result, findErr = cidr.FindAvailableCIDR(parentCidr, &mask, usedCidrs)
		if findErr != nil {
			diags = append(diags, diag.FromErr(findErr)...)
			return diags
		}
	}

	d.SetId(result.String())
	d.Set("result", result.String())

	return diags
}

func resourceAvailableCidrDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	d.SetId("")

	return diags
}
