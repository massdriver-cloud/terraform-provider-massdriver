package massdriver

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/stash"
)

func dataSourceStashEntries() *schema.Resource {
	// Terraform cannot mark one attribute of a computed list element as
	// sensitive, so sensitive values are left out of the list entirely.
	entry := stashEntryFields()
	delete(entry, "sensitive_value")
	entry["value"] = &schema.Schema{
		Description: "The entry's value. Empty for a sensitive entry: read it with the `massdriver_stash_entry` data source.",
		Type:        schema.TypeString,
		Computed:    true,
	}
	entry["key"] = &schema.Schema{
		Description: "The entry's key.",
		Type:        schema.TypeString,
		Computed:    true,
	}

	fields := map[string]*schema.Schema{
		"key_prefix": {
			Description: "Only list entries whose key starts with this text, such as `db/`. Terraform reads every entry in the stash and keeps the matching ones.",
			Type:        schema.TypeString,
			Optional:    true,
		},
		"keys": {
			Description: "The keys of the listed entries, sorted.",
			Type:        schema.TypeList,
			Computed:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
		},
		"entries": {
			Description: "The listed entries, sorted by key. Expired entries are not listed. The value of a sensitive entry is not listed.",
			Type:        schema.TypeList,
			Computed:    true,
			Elem:        &schema.Resource{Schema: entry},
		},
	}
	for name, s := range stashScopeSchema(false) {
		fields[name] = s
	}

	return &schema.Resource{
		Description: stashDataSourceAlphaNotice + "\n\n" +
			"Lists the entries in the Massdriver stash of a project, environment, component, instance, or deployment. " +
			"Stashes do not inherit: an instance's list does not hold its environment's entries.",

		ReadContext: dataSourceStashEntriesRead,

		Schema: fields,
	}
}

func dataSourceStashEntriesRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	scope, err := stashScopeFromData(d)
	if err != nil {
		return diag.FromErr(err)
	}
	prefix := d.Get("key_prefix").(string)

	var matched []stash.Entry
	for e, err := range pc.Stash.Iter(ctx, scope, stash.ListInput{}) {
		if err != nil {
			return diag.FromErr(err)
		}
		if strings.HasPrefix(e.Key, prefix) {
			matched = append(matched, e)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Key < matched[j].Key })

	keys := make([]string, 0, len(matched))
	entries := make([]map[string]any, 0, len(matched))
	for i := range matched {
		keys = append(keys, matched[i].Key)
		e := flattenStashEntry(&matched[i])
		delete(e, "sensitive_value")
		entries = append(entries, e)
	}

	if err := d.Set("keys", keys); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("entries", entries); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(scope.String() + "/" + prefix)
	return nil
}

func formatStashTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
