package massdriver

import (
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/stash"
)

func TestDataSourceStashEntryReadJSON(t *testing.T) {
	fake := &fakeStash{getResp: &stash.Entry{
		Key:       "md/v1/release",
		Value:     strPtr(`{ "version": "2026.11.27-5", "image": "visits:a41c9e2" }`),
		Format:    "JSON",
		UpdatedAt: time.Date(2026, 11, 27, 20, 3, 24, 0, time.UTC),
	}}
	d := schema.TestResourceDataRaw(t, dataSourceStashEntry().Schema, map[string]any{
		"instance": "ecomm-prod-api",
		"key":      "md/v1/release",
	})

	if diags := dataSourceStashEntryRead(t.Context(), d, &ProviderClient{Stash: fake}); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if d.Id() != "instance:ecomm-prod-api/md/v1/release" {
		t.Errorf("id = %q", d.Id())
	}
	want := map[string]any{
		"value":           `{ "version": "2026.11.27-5", "image": "visits:a41c9e2" }`,
		"data":            `{"image":"visits:a41c9e2","version":"2026.11.27-5"}`,
		"sensitive_value": "",
		"format":          "JSON",
		"expires_at":      "",
		"updated_at":      "2026-11-27T20:03:24Z",
	}
	for k, v := range want {
		if got := d.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// A sensitive value never lands in value or data, which show in plan output.
func TestDataSourceStashEntryReadSensitive(t *testing.T) {
	fake := &fakeStash{getResp: &stash.Entry{Key: "creds", Value: strPtr(`{"password":"hunter2"}`), Format: "JSON", Sensitive: true}}
	d := schema.TestResourceDataRaw(t, dataSourceStashEntry().Schema, map[string]any{
		"environment": "ecomm-prod",
		"key":         "creds",
	})

	if diags := dataSourceStashEntryRead(t.Context(), d, &ProviderClient{Stash: fake}); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if d.Get("value") != "" || d.Get("data") != "" {
		t.Errorf("value = %q, data = %q, want both empty", d.Get("value"), d.Get("data"))
	}
	if d.Get("sensitive_value") != `{"password":"hunter2"}` {
		t.Errorf("sensitive_value = %q", d.Get("sensitive_value"))
	}
}

func TestDataSourceStashEntryReadMissingFails(t *testing.T) {
	fake := &fakeStash{getErr: errors.New("get stash entry ami-id in instance:ecomm-prod-db: not found")}
	d := schema.TestResourceDataRaw(t, dataSourceStashEntry().Schema, map[string]any{
		"instance": "ecomm-prod-db",
		"key":      "ami-id",
	})
	if diags := dataSourceStashEntryRead(t.Context(), d, &ProviderClient{Stash: fake}); !diags.HasError() {
		t.Error("got no error, want the missing entry reported")
	}
}

func TestDataSourceStashEntriesFiltersByPrefix(t *testing.T) {
	fake := &fakeStash{entries: []stash.Entry{
		{Key: "db/port", Value: strPtr("5432"), Format: "TEXT"},
		{Key: "release", Value: strPtr("v1"), Format: "TEXT"},
		{Key: "db/hostname", Value: strPtr("db.example.com"), Format: "TEXT", Label: "Database host"},
	}}
	d := schema.TestResourceDataRaw(t, dataSourceStashEntries().Schema, map[string]any{
		"project":    "ecomm",
		"key_prefix": "db/",
	})

	if diags := dataSourceStashEntriesRead(t.Context(), d, &ProviderClient{Stash: fake}); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	keys := d.Get("keys").([]any)
	if len(keys) != 2 || keys[0] != "db/hostname" || keys[1] != "db/port" {
		t.Errorf("keys = %v, want [db/hostname db/port]", keys)
	}
	if got := d.Get("entries.0.label"); got != "Database host" {
		t.Errorf("entries.0.label = %q, want Database host", got)
	}
	if got := d.Get("entries.1.value"); got != "5432" {
		t.Errorf("entries.1.value = %q, want 5432", got)
	}
}

func TestDataSourceStashEntriesReportsListError(t *testing.T) {
	fake := &fakeStash{iterErr: errors.New("list stash entries in project:ecomm: forbidden")}
	d := schema.TestResourceDataRaw(t, dataSourceStashEntries().Schema, map[string]any{"project": "ecomm"})
	if diags := dataSourceStashEntriesRead(t.Context(), d, &ProviderClient{Stash: fake}); !diags.HasError() {
		t.Error("got no error, want the list error")
	}
}
