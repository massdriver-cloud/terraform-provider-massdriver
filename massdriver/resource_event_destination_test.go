package massdriver

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/eventdestinations"
)

// fakeEventDestinations applies changes the way the API does: update
// changes only the fields set on the input, and auth is kept but never
// returned.
type fakeEventDestinations struct {
	mu          sync.Mutex
	destination *eventdestinations.EventDestination
	auth        map[string]any
	getErr      error
	pauseErr    error

	creates []eventdestinations.CreateInput
	updates []eventdestinations.UpdateInput
	pauses  int
	resumes int
	deletes int
}

func (f *fakeEventDestinations) current() (*eventdestinations.EventDestination, error) {
	if f.destination == nil {
		return nil, fmt.Errorf("get event destination: %w", gql.ErrNotFound)
	}
	d := *f.destination
	d.EventTypes = slices.Clone(d.EventTypes)
	d.Attributes = maps.Clone(d.Attributes)
	return &d, nil
}

func (f *fakeEventDestinations) Get(_ context.Context, _ string) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.current()
}

func (f *fakeEventDestinations) Create(_ context.Context, input eventdestinations.CreateInput) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, input)
	f.auth = input.Auth
	f.destination = &eventdestinations.EventDestination{
		ID:            "ed-1",
		Name:          input.Name,
		Adapter:       input.Adapter,
		Config:        input.Config,
		EventTypes:    append([]string{}, input.EventTypes...),
		Attributes:    maps.Clone(input.Attributes),
		Condition:     input.Condition,
		Status:        string(eventdestinations.StatusActive),
		SigningSecret: "whsec_c2VjcmV0",
	}
	return f.current()
}

func (f *fakeEventDestinations) Update(_ context.Context, _ string, input eventdestinations.UpdateInput) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, input)
	if input.Name != nil {
		f.destination.Name = *input.Name
	}
	if input.Config != nil {
		f.destination.Config = input.Config
	}
	if input.Auth != nil {
		f.auth = input.Auth
	}
	if input.EventTypes != nil {
		f.destination.EventTypes = slices.Clone(*input.EventTypes)
	}
	if input.Attributes != nil {
		f.destination.Attributes = maps.Clone(input.Attributes)
	}
	if input.Condition != nil {
		f.destination.Condition = *input.Condition
	}
	return f.current()
}

func (f *fakeEventDestinations) Delete(_ context.Context, _ string) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	d, err := f.current()
	f.destination = nil
	return d, err
}

func (f *fakeEventDestinations) Pause(_ context.Context, _ string) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pauses++
	if f.pauseErr != nil {
		return nil, f.pauseErr
	}
	f.destination.Status = string(eventdestinations.StatusPaused)
	return f.current()
}

func (f *fakeEventDestinations) Resume(_ context.Context, _ string) (*eventdestinations.EventDestination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumes++
	f.destination.Status = string(eventdestinations.StatusActive)
	f.destination.FailingSince = time.Time{}
	return f.current()
}

// autoPause does what Massdriver does after a day of failed sends.
func (f *fakeEventDestinations) autoPause() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destination.Status = string(eventdestinations.StatusPaused)
	f.destination.FailingSince = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	f.destination.LastError = "HTTP 503"
}

func (f *fakeEventDestinations) check(fn func(f *fakeEventDestinations) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		return fn(f)
	}
}

func TestResourceEventDestinationRead(t *testing.T) {
	fake := &fakeEventDestinations{destination: &eventdestinations.EventDestination{
		ID:            "ed-1",
		Name:          "Production deployments",
		Adapter:       "HTTP",
		Config:        map[string]any{"url": "https://hooks.example.com/massdriver"},
		EventTypes:    []string{"deployment.*"},
		Attributes:    map[string]string{"md-environment": "prod"},
		Status:        "PAUSED",
		SigningSecret: "whsec_c2VjcmV0",
		FailingSince:  time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		LastError:     "HTTP 503",
	}}
	pc := &ProviderClient{EventDestinations: fake}

	rd := resourceEventDestination().TestResourceData()
	rd.SetId("ed-1")

	if diags := resourceEventDestinationRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	for attr, want := range map[string]any{
		"name":              "Production deployments",
		"adapter":           "HTTP",
		"config":            `{"url":"https://hooks.example.com/massdriver"}`,
		"paused":            true,
		"status":            "PAUSED",
		"signing_secret":    "whsec_c2VjcmV0",
		"failing_since":     "2026-11-20T10:00:00Z",
		"last_error":        "HTTP 503",
		"last_delivered_at": "",
		"condition":         "",
	} {
		if got := rd.Get(attr); got != want {
			t.Errorf("%s = %v, want %v", attr, got, want)
		}
	}
	if got := expandStringList(rd.Get("event_types").(*schema.Set).List()); !slices.Equal(got, []string{"deployment.*"}) {
		t.Errorf("event_types = %v", got)
	}
	if got := rd.Get("attributes").(map[string]any); got["md-environment"] != "prod" {
		t.Errorf("attributes = %v", got)
	}
}

func TestResourceEventDestinationReadNotFoundClearsID(t *testing.T) {
	pc := &ProviderClient{EventDestinations: &fakeEventDestinations{}}

	rd := resourceEventDestination().TestResourceData()
	rd.SetId("ed-1")

	if diags := resourceEventDestinationRead(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if rd.Id() != "" {
		t.Errorf("got id %q, want it cleared", rd.Id())
	}
}

func TestResourceEventDestinationReadPropagatesFailure(t *testing.T) {
	pc := &ProviderClient{EventDestinations: &fakeEventDestinations{getErr: errors.New("forbidden")}}

	rd := resourceEventDestination().TestResourceData()
	rd.SetId("ed-1")

	if diags := resourceEventDestinationRead(t.Context(), rd, pc); !diags.HasError() {
		t.Fatal("got no error, want forbidden")
	}
	if rd.Id() != "ed-1" {
		t.Errorf("a failed read must not clear the ID, got %q", rd.Id())
	}
}

func TestResourceEventDestinationDeleteAlreadyGone(t *testing.T) {
	pc := &ProviderClient{EventDestinations: &fakeEventDestinations{}}

	rd := resourceEventDestination().TestResourceData()
	rd.SetId("ed-1")

	if diags := resourceEventDestinationDelete(t.Context(), rd, pc); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestResourceEventDestinationAuthValidation(t *testing.T) {
	validate := resourceEventDestination().Schema["auth"].ValidateDiagFunc
	for value, ok := range map[string]bool{
		`{"headers":{"Authorization":"Bearer token"}}`: true,
		`{}`:            true,
		`Bearer s3cr3t`: false,
		`null`:          false,
		`["a"]`:         false,
	} {
		diags := validate(value, cty.GetAttrPath("auth"))
		if diags.HasError() == ok {
			t.Errorf("%s: got %v, want valid=%v", value, diags, ok)
		}
		for _, d := range diags {
			if strings.Contains(d.Summary+d.Detail, "s3cr3t") {
				t.Errorf("diagnostic echoes the auth value: %v", d)
			}
		}
	}
}

// Drives the real terraform binary against a fake API to check what each
// plan sends: auth only on create and when auth_version changes, partial
// updates, and an automatic pause showing as drift that apply resumes.
func TestResourceEventDestinationLifecycle(t *testing.T) {
	fake := &fakeEventDestinations{}
	pc := &ProviderClient{EventDestinations: fake}
	const addr = "massdriver_event_destination.this"

	config := func(name, auth string, authVersion int, extra string) string {
		authLines := ""
		if auth != "" {
			authLines = fmt.Sprintf("  auth         = jsonencode(%s)\n", auth)
		}
		return fmt.Sprintf(`
resource "massdriver_event_destination" "this" {
  name         = %q
  adapter      = "HTTP"
  config       = jsonencode({ url = "https://hooks.example.com/massdriver" })
%s  auth_version = %d
  event_types  = ["deployment.*"]
%s
}`, name, authLines, authVersion, extra)
	}
	bearer := `{ headers = { Authorization = "Bearer one" } }`

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: fakeProviderFactories(pc),
		Steps: []resource.TestStep{
			{
				Config: config("Production deployments", bearer, 1, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "ed-1"),
					resource.TestCheckResourceAttr(addr, "status", "ACTIVE"),
					resource.TestCheckResourceAttr(addr, "paused", "false"),
					resource.TestCheckResourceAttr(addr, "signing_secret", "whsec_c2VjcmV0"),
					resource.TestCheckNoResourceAttr(addr, "auth"),
					fake.check(func(f *fakeEventDestinations) error {
						if len(f.creates) != 1 || f.creates[0].Auth["headers"] == nil {
							return fmt.Errorf("creates = %+v, want one with auth", f.creates)
						}
						return nil
					}),
				),
			},
			// Same auth_version: auth is not sent again.
			{
				Config: config("Renamed", bearer, 1, `  attributes = { "md-environment" = "prod" }`),
				Check: fake.check(func(f *fakeEventDestinations) error {
					if len(f.updates) != 1 {
						return fmt.Errorf("got %d updates, want 1", len(f.updates))
					}
					in := f.updates[0]
					if in.Name == nil || *in.Name != "Renamed" || in.Auth != nil || in.Config != nil || in.EventTypes != nil || in.Condition != nil {
						return fmt.Errorf("update sent %+v, want only name and attributes", in)
					}
					if in.Attributes["md-environment"] != "prod" {
						return fmt.Errorf("update sent attributes %v", in.Attributes)
					}
					return nil
				}),
			},
			// A new auth_version sends auth again.
			{
				Config: config("Renamed", `{ headers = { Authorization = "Bearer two" } }`, 2, `  attributes = { "md-environment" = "prod" }`),
				Check: fake.check(func(f *fakeEventDestinations) error {
					in := f.updates[len(f.updates)-1]
					headers, _ := in.Auth["headers"].(map[string]any)
					if len(f.updates) != 2 || headers["Authorization"] != "Bearer two" || in.Name != nil {
						return fmt.Errorf("updates = %+v, want a second with only the new auth", f.updates)
					}
					return nil
				}),
			},
			// Massdriver pauses the destination; the plan shows it and apply resumes it.
			{
				PreConfig: fake.autoPause,
				Config:    config("Renamed", `{ headers = { Authorization = "Bearer two" } }`, 2, `  attributes = { "md-environment" = "prod" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "paused", "false"),
					resource.TestCheckResourceAttr(addr, "status", "ACTIVE"),
					resource.TestCheckResourceAttr(addr, "failing_since", ""),
					fake.check(func(f *fakeEventDestinations) error {
						if f.resumes != 1 || len(f.updates) != 2 {
							return fmt.Errorf("got %d resumes and %d updates, want 1 resume and no update", f.resumes, len(f.updates))
						}
						return nil
					}),
				),
			},
			// Removing auth and bumping auth_version removes the credentials;
			// clearing event_types receives every event.
			{
				Config: `
resource "massdriver_event_destination" "this" {
  name         = "Renamed"
  adapter      = "HTTP"
  config       = jsonencode({ url = "https://hooks.example.com/massdriver" })
  auth_version = 3
  attributes   = { "md-environment" = "prod" }
  paused       = true
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "status", "PAUSED"),
					fake.check(func(f *fakeEventDestinations) error {
						in := f.updates[len(f.updates)-1]
						if len(f.updates) != 3 || in.Auth == nil || len(in.Auth) != 0 {
							return fmt.Errorf("update sent auth %v, want {}", in.Auth)
						}
						if in.EventTypes == nil || len(*in.EventTypes) != 0 {
							return fmt.Errorf("update sent event types %v, want []", in.EventTypes)
						}
						if f.pauses != 1 {
							return fmt.Errorf("got %d pauses, want 1", f.pauses)
						}
						return nil
					}),
				),
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateId:           "ed-1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"auth_version"},
			},
		},
	})
}

func TestResourceEventDestinationCreatePaused(t *testing.T) {
	fake := &fakeEventDestinations{}
	pc := &ProviderClient{EventDestinations: fake}

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: fakeProviderFactories(pc),
		Steps: []resource.TestStep{{
			Config: `
resource "massdriver_event_destination" "this" {
  name    = "Audit"
  adapter = "HTTP"
  config  = jsonencode({ url = "https://hooks.example.com/massdriver" })
  paused  = true
}`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("massdriver_event_destination.this", "status", "PAUSED"),
				fake.check(func(f *fakeEventDestinations) error {
					if f.pauses != 1 || f.creates[0].Auth != nil {
						return fmt.Errorf("got %d pauses and auth %v, want 1 pause and no auth", f.pauses, f.creates[0].Auth)
					}
					return nil
				}),
			),
		}},
	})
}

// A failed pause after create must still record the destination in state,
// so it is not left behind untracked.
func TestResourceEventDestinationCreatePauseFails(t *testing.T) {
	fake := &fakeEventDestinations{pauseErr: errors.New("pause event destination: forbidden")}
	pc := &ProviderClient{EventDestinations: fake}

	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: fakeProviderFactories(pc),
		Steps: []resource.TestStep{{
			Config: `
resource "massdriver_event_destination" "this" {
  name    = "Audit"
  adapter = "HTTP"
  config  = jsonencode({ url = "https://hooks.example.com/massdriver" })
  paused  = true
}`,
			ExpectError: regexp.MustCompile("forbidden"),
		}},
		CheckDestroy: func(*terraform.State) error {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if fake.deletes != 1 {
				return fmt.Errorf("got %d deletes, want the created destination destroyed", fake.deletes)
			}
			return nil
		},
	})
}
