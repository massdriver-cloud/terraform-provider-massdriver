package massdriver

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/organizations"
)

type fakeCustomAttributes struct {
	attrs   []organizations.CustomAttribute
	iterErr error

	createResp *organizations.CustomAttribute
	createErr  error
	updateResp *organizations.CustomAttribute
	updateErr  error
	deleteErr  error

	createInput organizations.CreateCustomAttributeInput
	updateID    string
	updateInput organizations.UpdateCustomAttributeInput
	deleteID    string
}

func (f *fakeCustomAttributes) IterCustomAttributes(_ context.Context, _ organizations.ListCustomAttributesInput) iter.Seq2[organizations.CustomAttribute, error] {
	return func(yield func(organizations.CustomAttribute, error) bool) {
		if f.iterErr != nil {
			yield(organizations.CustomAttribute{}, f.iterErr)
			return
		}
		for _, a := range f.attrs {
			if !yield(a, nil) {
				return
			}
		}
	}
}
func (f *fakeCustomAttributes) CreateCustomAttribute(_ context.Context, input organizations.CreateCustomAttributeInput) (*organizations.CustomAttribute, error) {
	f.createInput = input
	return f.createResp, f.createErr
}
func (f *fakeCustomAttributes) UpdateCustomAttribute(_ context.Context, id string, input organizations.UpdateCustomAttributeInput) (*organizations.CustomAttribute, error) {
	f.updateID = id
	f.updateInput = input
	return f.updateResp, f.updateErr
}
func (f *fakeCustomAttributes) DeleteCustomAttribute(_ context.Context, id string) (*organizations.CustomAttribute, error) {
	f.deleteID = id
	return nil, f.deleteErr
}

var teamAttr = organizations.CustomAttribute{ID: "ca-1", Key: "team", Scope: "PROJECT", Required: false, Values: []string{"platform", "data"}}

func TestResourceCustomAttributeCreate(t *testing.T) {
	created := teamAttr
	fake := &fakeCustomAttributes{createResp: &created}
	pc := &ProviderClient{CustomAttributes: fake}

	rd := schema.TestResourceDataRaw(t, resourceCustomAttribute().Schema, map[string]any{
		"key":    "team",
		"scope":  "PROJECT",
		"values": []any{"platform", "data"},
	})

	if diags := resourceCustomAttributeCreate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if rd.Id() != "ca-1" {
		t.Errorf("got id %q, want ca-1", rd.Id())
	}
	in := fake.createInput
	if in.Key != "team" || in.Scope != organizations.AttributeScopeProject || !slices.Equal(in.Values, []string{"platform", "data"}) {
		t.Errorf("got create input %+v", in)
	}
	// The platform defaults required to true, so the provider's false default
	// must be sent explicitly.
	if in.Required == nil || *in.Required {
		t.Errorf("got required %v, want explicit false", in.Required)
	}
}

func TestResourceCustomAttributeCreatePropagatesAPIFailure(t *testing.T) {
	fake := &fakeCustomAttributes{createErr: errors.New("create custom attribute team: key has already been taken")}
	pc := &ProviderClient{CustomAttributes: fake}

	rd := schema.TestResourceDataRaw(t, resourceCustomAttribute().Schema, map[string]any{
		"key": "team", "scope": "PROJECT", "values": []any{"a"},
	})

	diags := resourceCustomAttributeCreate(t.Context(), rd, pc)
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "already been taken") {
		t.Fatalf("got %v, want the API error", diags)
	}
	if rd.Id() != "" {
		t.Errorf("got id %q, want none", rd.Id())
	}
}

func TestResourceCustomAttributeRead(t *testing.T) {
	other := organizations.CustomAttribute{ID: "ca-0", Key: "env", Scope: "ENVIRONMENT", Values: []string{"prod"}}
	attr := teamAttr
	attr.Required = true
	fake := &fakeCustomAttributes{attrs: []organizations.CustomAttribute{other, attr}}
	pc := &ProviderClient{CustomAttributes: fake}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Get("key") != "team" || rd.Get("scope") != "PROJECT" || rd.Get("required") != true {
		t.Errorf("got key=%v scope=%v required=%v", rd.Get("key"), rd.Get("scope"), rd.Get("required"))
	}
	if got := expandStringList(rd.Get("values").([]any)); !slices.Equal(got, []string{"platform", "data"}) {
		t.Errorf("got values %v", got)
	}
}

func TestResourceCustomAttributeReadGoneClearsID(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{}}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("got id %q, want it cleared", rd.Id())
	}
}

func TestResourceCustomAttributeReadPropagatesListFailure(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{iterErr: errors.New("boom")}}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeRead(t.Context(), rd, pc); !diags.HasError() {
		t.Fatal("got no error, want the list failure")
	}
	if rd.Id() != "ca-1" {
		t.Errorf("a failed list must not clear the ID, got %q", rd.Id())
	}
}

func TestResourceCustomAttributeUpdate(t *testing.T) {
	updated := teamAttr
	updated.Required = true
	updated.Values = []string{"platform", "data", "ml"}
	fake := &fakeCustomAttributes{updateResp: &updated}
	pc := &ProviderClient{CustomAttributes: fake}

	rd := schema.TestResourceDataRaw(t, resourceCustomAttribute().Schema, map[string]any{
		"key": "team", "scope": "PROJECT", "required": true, "values": []any{"platform", "data", "ml"},
	})
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeUpdate(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.updateID != "ca-1" {
		t.Errorf("got update id %q, want ca-1", fake.updateID)
	}
	if fake.updateInput.Required == nil || !*fake.updateInput.Required || !slices.Equal(fake.updateInput.Values, []string{"platform", "data", "ml"}) {
		t.Errorf("got update input %+v", fake.updateInput)
	}
}

func TestResourceCustomAttributeDelete(t *testing.T) {
	fake := &fakeCustomAttributes{}
	pc := &ProviderClient{CustomAttributes: fake}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.deleteID != "ca-1" || rd.Id() != "" {
		t.Errorf("got delete id %q, state id %q", fake.deleteID, rd.Id())
	}
}

func TestResourceCustomAttributeDeleteAlreadyGone(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{deleteErr: errors.New("delete custom attribute: failed")}}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestResourceCustomAttributeDeleteFailureWhileStillPresent(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{
		attrs:     []organizations.CustomAttribute{teamAttr},
		deleteErr: errors.New("delete custom attribute: forbidden"),
	}}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("ca-1")

	if diags := resourceCustomAttributeDelete(t.Context(), rd, pc); !diags.HasError() {
		t.Fatal("got no error, want the delete failure")
	}
}

func TestResourceCustomAttributeImport(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{attrs: []organizations.CustomAttribute{teamAttr}}}

	for _, id := range []string{"ca-1", "team", "TEAM"} {
		t.Run(id, func(t *testing.T) {
			rd := resourceCustomAttribute().TestResourceData()
			rd.SetId(id)

			got, err := resourceCustomAttributeImport(t.Context(), rd, pc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[0].Id() != "ca-1" {
				t.Errorf("got id %q, want ca-1", got[0].Id())
			}
		})
	}
}

func TestResourceCustomAttributeImportUnknown(t *testing.T) {
	pc := &ProviderClient{CustomAttributes: &fakeCustomAttributes{attrs: []organizations.CustomAttribute{teamAttr}}}

	rd := resourceCustomAttribute().TestResourceData()
	rd.SetId("nope")

	if _, err := resourceCustomAttributeImport(t.Context(), rd, pc); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("got %v, want a not-found error naming the ID", err)
	}
}

func TestResourceCustomAttributeKeyValidation(t *testing.T) {
	validate := resourceCustomAttribute().Schema["key"].ValidateFunc
	for key, ok := range map[string]bool{
		"team":                  true,
		"_cost_center":          true,
		"Team2":                 true,
		"2team":                 false,
		"md-owner":              false,
		"cost-center":           false,
		"":                      false,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
	} {
		_, errs := validate(key, "key")
		if (len(errs) == 0) != ok {
			t.Errorf("key %q: got errors %v, want valid=%v", key, errs, ok)
		}
	}
}
