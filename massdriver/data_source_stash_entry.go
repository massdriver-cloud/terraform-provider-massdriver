package massdriver

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/stash"
)

const stashDataSourceAlphaNotice = "Alpha: this data source can change or be removed at any time."

// stashEntryFields are the computed attributes of one entry, shared by both
// stash data sources.
func stashEntryFields() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"value": {
			Description: "The entry's value. Empty when the entry is sensitive; see `sensitive_value`.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"sensitive_value": {
			Description: "The value of a sensitive entry. Empty when the entry is not sensitive, or when the caller cannot read sensitive values. Reading it needs `stash:export` or the deployment's own token.",
			Type:        schema.TypeString,
			Computed:    true,
			Sensitive:   true,
		},
		"data": {
			Description: "The value of a `JSON` entry as a JSON object string, for use with `jsondecode`. Empty for a `TEXT` entry and for a sensitive entry.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"format": {
			Description: "How the value is read: `TEXT` or `JSON`.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"label": {
			Description: "The entry's label. Empty when it has none.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"immutable": {
			Description: "Whether the server refuses a second write to the key.",
			Type:        schema.TypeBool,
			Computed:    true,
		},
		"sensitive": {
			Description: "Whether the entry is sensitive.",
			Type:        schema.TypeBool,
			Computed:    true,
		},
		"expires_at": {
			Description: "When the entry expires, in RFC 3339 format. Empty when the entry does not expire.",
			Type:        schema.TypeString,
			Computed:    true,
		},
		"updated_at": {
			Description: "When the entry was last written, in RFC 3339 format.",
			Type:        schema.TypeString,
			Computed:    true,
		},
	}
}

func dataSourceStashEntry() *schema.Resource {
	fields := stashEntryFields()
	fields["key"] = stashKeySchema(false)
	for name, s := range stashScopeSchema(false) {
		fields[name] = s
	}

	return &schema.Resource{
		Description: stashDataSourceAlphaNotice + "\n\n" +
			"Reads one entry from the Massdriver stash of a project, environment, component, instance, or deployment. " +
			"Fails when the entry does not exist or has expired. Stashes do not inherit: to read an environment's entry, set `environment`, not `instance`.",

		ReadContext: dataSourceStashEntryRead,

		Schema: fields,
	}
}

func dataSourceStashEntryRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	scope, err := stashScopeFromData(d)
	if err != nil {
		return diag.FromErr(err)
	}
	key := d.Get("key").(string)

	entry, err := pc.Stash.Get(ctx, scope, key)
	if err != nil {
		return diag.FromErr(err)
	}

	for k, v := range flattenStashEntry(entry) {
		if err := d.Set(k, v); err != nil {
			return diag.FromErr(err)
		}
	}
	d.SetId(stashEntryID(scope, key))
	return nil
}

// flattenStashEntry maps an entry to the stashEntryFields attributes, plus
// `key`. A sensitive value goes only to `sensitive_value`, and its `data` is
// left empty so the value never lands in a non-sensitive attribute.
func flattenStashEntry(e *stash.Entry) map[string]any {
	out := map[string]any{
		"key":             e.Key,
		"value":           "",
		"sensitive_value": "",
		"data":            "",
		"format":          e.Format,
		"label":           e.Label,
		"immutable":       e.Immutable,
		"sensitive":       e.Sensitive,
		"expires_at":      formatStashTime(e.ExpiresAt),
		"updated_at":      formatStashTime(e.UpdatedAt),
	}
	if e.Value == nil {
		return out
	}
	if e.Sensitive {
		out["sensitive_value"] = *e.Value
		return out
	}
	out["value"] = *e.Value
	if e.Format == string(stash.FormatJSON) {
		if obj, ok := parseJSONObject(*e.Value); ok {
			if b, err := json.Marshal(obj); err == nil {
				out["data"] = string(b)
			}
		}
	}
	return out
}
