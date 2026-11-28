package massdriver

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/stash"
)

type fakeStash struct {
	getResp *stash.Entry
	getErr  error
	setResp *stash.Entry
	setErr  error
	delErr  error
	entries []stash.Entry
	iterErr error

	setScope stash.Scope
	setKey   string
	setInput stash.SetInput
	delScope stash.Scope
	delKey   string

	setCalls, delCalls int
}

func (f *fakeStash) Get(_ context.Context, _ stash.Scope, _ string) (*stash.Entry, error) {
	return f.getResp, f.getErr
}

func (f *fakeStash) Iter(_ context.Context, _ stash.Scope, _ stash.ListInput) iter.Seq2[stash.Entry, error] {
	return func(yield func(stash.Entry, error) bool) {
		if f.iterErr != nil {
			yield(stash.Entry{}, f.iterErr)
			return
		}
		for _, e := range f.entries {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (f *fakeStash) Set(_ context.Context, scope stash.Scope, key string, input stash.SetInput) (*stash.Entry, error) {
	f.setScope, f.setKey, f.setInput = scope, key, input
	f.setCalls++
	if f.setErr != nil {
		return nil, f.setErr
	}
	if f.setResp != nil {
		return f.setResp, nil
	}
	return &stash.Entry{Key: key, Value: &input.Value, Format: string(input.Format), Label: input.Label, Immutable: input.Immutable, Sensitive: input.Sensitive}, nil
}

func (f *fakeStash) Delete(_ context.Context, scope stash.Scope, key string) error {
	f.delScope, f.delKey = scope, key
	f.delCalls++
	return f.delErr
}

func strPtr(s string) *string { return &s }

func TestStashEntryScopeValidation(t *testing.T) {
	cases := map[string]struct {
		config  map[string]any
		wantErr bool
	}{
		"one scope": {
			config: map[string]any{"instance": "ecomm-prod-db", "key": "release", "value": "v1"},
		},
		"no scope": {
			config:  map[string]any{"key": "release", "value": "v1"},
			wantErr: true,
		},
		"two scopes": {
			config:  map[string]any{"project": "ecomm", "instance": "ecomm-prod-db", "key": "release", "value": "v1"},
			wantErr: true,
		},
		"value and sensitive_value": {
			config:  map[string]any{"project": "ecomm", "key": "release", "value": "v1", "sensitive_value": "v1"},
			wantErr: true,
		},
		"key with a space": {
			config:  map[string]any{"project": "ecomm", "key": "my key", "value": "v1"},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for label, r := range map[string]*schema.Resource{
				"resource":    resourceStashEntry(),
				"data source": dataSourceStashEntry(),
			} {
				cfg := map[string]any{}
				for k, v := range tc.config {
					if s, ok := r.Schema[k]; ok && (s.Required || s.Optional) {
						cfg[k] = v
					}
				}
				if label == "data source" && name == "value and sensitive_value" {
					continue
				}
				diags := r.Validate(terraform.NewResourceConfigRaw(cfg))
				if diags.HasError() != tc.wantErr {
					t.Errorf("%s: got errors %v, wantErr %v", label, diags, tc.wantErr)
				}
			}
		})
	}
}

func TestParseStashEntryID(t *testing.T) {
	cases := []struct {
		id        string
		wantScope stash.Scope
		wantKey   string
		wantErr   bool
	}{
		{id: "project:ecomm/release", wantScope: stash.Scope{Kind: stash.ScopeProject, ID: "ecomm"}, wantKey: "release"},
		{id: "environment:ecomm-prod/md/v1/links", wantScope: stash.Scope{Kind: stash.ScopeEnvironment, ID: "ecomm-prod"}, wantKey: "md/v1/links"},
		{
			id:        "deployment:4f6e2a3c-0d3e-4c1a-9a43-2c8f5b1e7d90/db/hostname",
			wantScope: stash.Scope{Kind: stash.ScopeDeployment, ID: "4f6e2a3c-0d3e-4c1a-9a43-2c8f5b1e7d90"},
			wantKey:   "db/hostname",
		},
		{id: "release", wantErr: true},
		{id: "project:ecomm", wantErr: true},
		{id: "project:/release", wantErr: true},
		{id: "project:ecomm/", wantErr: true},
		{id: "bucket:ecomm/release", wantErr: true},
		{id: "project:ecomm/bad key", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			scope, key, err := parseStashEntryID(tc.id)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("got scope %v key %q, want an error", scope, key)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scope != tc.wantScope || key != tc.wantKey {
				t.Errorf("got %v %q, want %v %q", scope, key, tc.wantScope, tc.wantKey)
			}
			if got := stashEntryID(scope, key); got != tc.id {
				t.Errorf("stashEntryID round trip: got %q, want %q", got, tc.id)
			}
		})
	}
}

func TestJSONEquivalent(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`{"a":1,"b":2}`, `{ "b": 2, "a": 1 }`, true},
		{`{"a":{"x":[1,2]}}`, "{\n  \"a\": {\"x\": [1, 2]}\n}", true},
		{`{"n":12345678901234567890}`, `{"n":12345678901234567891}`, false},
		{`{"a":1}`, `{"a":2}`, false},
		{`{"a":[1,2]}`, `{"a":[2,1]}`, false},
		{`[1]`, `[1]`, false},
		{`not json`, `not json`, false},
		{`{"a":1} {"b":2}`, `{"a":1}`, false},
	}

	for _, tc := range cases {
		if got := jsonEquivalent(tc.a, tc.b); got != tc.want {
			t.Errorf("jsonEquivalent(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestStashEntryCreate(t *testing.T) {
	// Get fails as if a short TTL had already expired: state comes from
	// the write, so create still succeeds.
	fake := &fakeStash{getErr: gql.ErrNotFound}
	pc := &ProviderClient{Stash: fake}

	d := schema.TestResourceDataRaw(t, resourceStashEntry().Schema, map[string]any{
		"instance": "ecomm-prod-db",
		"key":      "db/hostname",
		"value":    "db.example.com",
		"label":    "Database host",
		"ttl":      600,
	})

	if diags := resourceStashEntryCreate(t.Context(), d, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if d.Id() != "instance:ecomm-prod-db/db/hostname" {
		t.Errorf("got id %q, want instance:ecomm-prod-db/db/hostname", d.Id())
	}
	if fake.setScope != (stash.Scope{Kind: stash.ScopeInstance, ID: "ecomm-prod-db"}) || fake.setKey != "db/hostname" {
		t.Errorf("got Set scope %v key %q", fake.setScope, fake.setKey)
	}
	want := stash.SetInput{Value: "db.example.com", Label: "Database host", Format: "TEXT", TTL: 600 * time.Second}
	if fake.setInput != want {
		t.Errorf("got SetInput %+v, want %+v", fake.setInput, want)
	}
	if got := d.Get("value"); got != "db.example.com" {
		t.Errorf("got value %q, want db.example.com", got)
	}
}

func TestStashEntryCreateSensitive(t *testing.T) {
	fake := &fakeStash{setResp: &stash.Entry{Key: "kubeconfig", Value: nil, Format: "TEXT", Sensitive: true}}
	pc := &ProviderClient{Stash: fake}

	d := schema.TestResourceDataRaw(t, resourceStashEntry().Schema, map[string]any{
		"environment":     "ecomm-prod",
		"key":             "kubeconfig",
		"sensitive_value": "secret-kubeconfig",
	})

	if diags := resourceStashEntryCreate(t.Context(), d, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !fake.setInput.Sensitive || fake.setInput.Value != "secret-kubeconfig" {
		t.Errorf("got SetInput %+v, want the sensitive value sent with Sensitive true", fake.setInput)
	}
	if got := d.Get("sensitive_value"); got != "secret-kubeconfig" {
		t.Errorf("got sensitive_value %q, want the configured value kept when the server hides it", got)
	}
	if got := d.Get("value"); got != "" {
		t.Errorf("got value %q, want empty", got)
	}
	if got := d.Get("sensitive"); got != true {
		t.Errorf("got sensitive %v, want true", got)
	}
}

// A sensitive value the caller can read goes to sensitive_value only.
func TestStashEntryReadSensitiveValue(t *testing.T) {
	fake := &fakeStash{getResp: &stash.Entry{Key: "kubeconfig", Value: strPtr("rotated"), Format: "TEXT", Sensitive: true}}
	pc := &ProviderClient{Stash: fake}

	d := schema.TestResourceDataRaw(t, resourceStashEntry().Schema, map[string]any{
		"environment":     "ecomm-prod",
		"key":             "kubeconfig",
		"sensitive_value": "original",
	})
	d.SetId("environment:ecomm-prod/kubeconfig")

	if diags := resourceStashEntryRead(t.Context(), d, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := d.Get("sensitive_value"); got != "rotated" {
		t.Errorf("got sensitive_value %q, want rotated", got)
	}
	if got := d.Get("value"); got != "" {
		t.Errorf("got value %q, want empty", got)
	}
}

func TestStashEntryReadMissingClearsState(t *testing.T) {
	fake := &fakeStash{getErr: gql.ErrNotFound}
	pc := &ProviderClient{Stash: fake}

	d := schema.TestResourceDataRaw(t, resourceStashEntry().Schema, map[string]any{
		"instance": "ecomm-prod-db",
		"key":      "ami-id",
		"value":    "ami-123",
	})
	d.SetId("instance:ecomm-prod-db/ami-id")

	if diags := resourceStashEntryRead(t.Context(), d, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("got id %q, want empty so the next plan writes the entry again", d.Id())
	}
}

func TestStashEntryDelete(t *testing.T) {
	cases := map[string]struct {
		fake    *fakeStash
		wantErr bool
	}{
		"deleted":         {fake: &fakeStash{}},
		"already gone":    {fake: &fakeStash{delErr: &gql.MutationFailedError{Op: "delete stash entry"}, getErr: gql.ErrNotFound}},
		"refused":         {fake: &fakeStash{delErr: errors.New("forbidden"), getResp: &stash.Entry{Key: "release"}}, wantErr: true},
		"get also failed": {fake: &fakeStash{delErr: errors.New("boom"), getErr: errors.New("boom")}, wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := schema.TestResourceDataRaw(t, resourceStashEntry().Schema, map[string]any{
				"project": "ecomm",
				"key":     "release",
				"value":   "v1",
			})
			d.SetId("project:ecomm/release")

			diags := resourceStashEntryDelete(t.Context(), d, &ProviderClient{Stash: tc.fake})
			if diags.HasError() != tc.wantErr {
				t.Fatalf("got diagnostics %v, wantErr %v", diags, tc.wantErr)
			}
			if tc.fake.delScope != (stash.Scope{Kind: stash.ScopeProject, ID: "ecomm"}) || tc.fake.delKey != "release" {
				t.Errorf("got Delete scope %v key %q", tc.fake.delScope, tc.fake.delKey)
			}
		})
	}
}

func TestStashEntryImport(t *testing.T) {
	r := resourceStashEntry()
	d := r.TestResourceData()
	d.SetId("environment:ecomm-prod/md/v1/links")

	out, err := r.Importer.StateContext(t.Context(), d, &ProviderClient{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out[0]
	if got.Get("environment") != "ecomm-prod" || got.Get("key") != "md/v1/links" {
		t.Errorf("got environment %q key %q", got.Get("environment"), got.Get("key"))
	}

	d.SetId("ecomm-prod/md/v1/links")
	if _, err := r.Importer.StateContext(t.Context(), d, &ProviderClient{}); err == nil || !strings.Contains(err.Error(), "<kind>:<scope-id>/<key>") {
		t.Errorf("got %v, want the expected ID format in the error", err)
	}
}

// planStashEntry plans config against an existing entry whose state is attrs.
func planStashEntry(t *testing.T, attrs map[string]string, config map[string]any) *terraform.InstanceDiff {
	t.Helper()
	r := resourceStashEntry()
	state := &terraform.InstanceState{ID: "project:ecomm/release", Attributes: attrs}
	diff, err := r.Diff(t.Context(), state, terraform.NewResourceConfigRaw(config), &ProviderClient{})
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}
	return diff
}

func baseStashState(overrides map[string]string) map[string]string {
	attrs := map[string]string{
		"id":              "project:ecomm/release",
		"project":         "ecomm",
		"key":             "release",
		"value":           "v1",
		"sensitive_value": "",
		"format":          "TEXT",
		"label":           "",
		"ttl":             "0",
		"immutable":       "false",
		"sensitive":       "false",
	}
	for k, v := range overrides {
		attrs[k] = v
	}
	return attrs
}

func TestStashEntryPlanUpdatesMutableEntryInPlace(t *testing.T) {
	diff := planStashEntry(t, baseStashState(nil), map[string]any{"project": "ecomm", "key": "release", "value": "v2"})
	if diff == nil || diff.Attributes["value"] == nil {
		t.Fatalf("got diff %v, want a change to value", diff)
	}
	if diff.RequiresNew() {
		t.Error("a mutable entry should update in place")
	}
}

func TestStashEntryPlanReplacesImmutableEntry(t *testing.T) {
	diff := planStashEntry(t, baseStashState(map[string]string{"immutable": "true"}),
		map[string]any{"project": "ecomm", "key": "release", "value": "v2", "immutable": true})
	if diff == nil || !diff.RequiresNew() {
		t.Fatalf("got diff %v, want a replacement: the server refuses a write to an immutable entry", diff)
	}
}

// A mutable entry can become immutable in place: the write that sets the
// flag is not refused.
func TestStashEntryPlanMakesEntryImmutableInPlace(t *testing.T) {
	diff := planStashEntry(t, baseStashState(nil),
		map[string]any{"project": "ecomm", "key": "release", "value": "v1", "immutable": true})
	if diff == nil || diff.RequiresNew() {
		t.Fatalf("got diff %v, want an in-place update", diff)
	}
}

func TestStashEntryPlanChangingKeyReplaces(t *testing.T) {
	diff := planStashEntry(t, baseStashState(nil), map[string]any{"project": "ecomm", "key": "release2", "value": "v1"})
	if diff == nil || !diff.RequiresNew() {
		t.Fatalf("got diff %v, want a replacement", diff)
	}
}

func TestStashEntryPlanIgnoresJSONFormatting(t *testing.T) {
	diff := planStashEntry(t,
		baseStashState(map[string]string{"format": "JSON", "value": `{"version":"1.2.3","image":"api:abc"}`}),
		map[string]any{"project": "ecomm", "key": "release", "format": "JSON", "value": "{\n  \"image\": \"api:abc\",\n  \"version\": \"1.2.3\"\n}"})
	if diff != nil && diff.Attributes["value"] != nil {
		t.Errorf("got a value change %v, want none for equivalent JSON", diff.Attributes["value"])
	}
}

func TestStashEntryPlanShowsTextWhitespaceChange(t *testing.T) {
	diff := planStashEntry(t, baseStashState(map[string]string{"value": `{"a":1}`}),
		map[string]any{"project": "ecomm", "key": "release", "value": `{ "a": 1 }`})
	if diff == nil || diff.Attributes["value"] == nil {
		t.Error("a TEXT value is compared as written")
	}
}

func TestStashEntryPlanRejectsJSONThatIsNotAnObject(t *testing.T) {
	r := resourceStashEntry()
	_, err := r.Diff(t.Context(), nil, terraform.NewResourceConfigRaw(map[string]any{
		"project": "ecomm", "key": "release", "format": "JSON", "value": `["a"]`,
	}), &ProviderClient{})
	if err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Errorf("got %v, want the JSON object error", err)
	}
}

func TestStashEntryPlanMarksSensitive(t *testing.T) {
	diff := planStashEntry(t, baseStashState(nil), map[string]any{"project": "ecomm", "key": "release", "sensitive_value": "v1"})
	if diff == nil || diff.Attributes["sensitive"] == nil || diff.Attributes["sensitive"].New != "true" {
		t.Fatalf("got diff %v, want sensitive planned as true", diff)
	}
}
