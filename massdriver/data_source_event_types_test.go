package massdriver

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/eventdestinations"
)

type fakeEventTypes struct {
	eventTypes []eventdestinations.EventType
	err        error
	input      eventdestinations.ListEventTypesInput
}

func (f *fakeEventTypes) ListEventTypes(_ context.Context, input eventdestinations.ListEventTypesInput) ([]eventdestinations.EventType, error) {
	f.input = input
	return f.eventTypes, f.err
}

func TestDataSourceEventTypesRead(t *testing.T) {
	fake := &fakeEventTypes{eventTypes: []eventdestinations.EventType{
		{ID: "deployment.completed", Description: "A deployment finished.", Entity: "deployment", Action: "completed", ChangeType: "UPDATED"},
		{ID: "deployment.started", Description: "A deployment started.", Entity: "deployment", Action: "started", ChangeType: "UPDATED"},
	}}
	pc := &ProviderClient{EventTypes: fake}

	rd := dataSourceEventTypes().TestResourceData()
	if err := rd.Set("entity", "deployment"); err != nil {
		t.Fatal(err)
	}

	if diags := dataSourceEventTypesRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if fake.input.Entity != "deployment" {
		t.Errorf("got entity %q, want deployment", fake.input.Entity)
	}
	if rd.Id() != "event_types/deployment" {
		t.Errorf("got id %q", rd.Id())
	}
	if got := expandStringList(rd.Get("ids").([]any)); !slices.Equal(got, []string{"deployment.completed", "deployment.started"}) {
		t.Errorf("ids = %v", got)
	}
	if rd.Get("event_types.0.change_type") != "UPDATED" || rd.Get("event_types.1.action") != "started" {
		t.Errorf("event_types = %v", rd.Get("event_types"))
	}
}

func TestDataSourceEventTypesReadPropagatesFailure(t *testing.T) {
	pc := &ProviderClient{EventTypes: &fakeEventTypes{err: errors.New("forbidden")}}

	if diags := dataSourceEventTypesRead(t.Context(), dataSourceEventTypes().TestResourceData(), pc); !diags.HasError() {
		t.Fatal("got no error, want forbidden")
	}
}
