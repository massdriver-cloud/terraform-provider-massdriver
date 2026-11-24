package massdriver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/eventdestinations"
)

// eventDestinationsAPI is the slice of *eventdestinations.Service this
// resource calls.
type eventDestinationsAPI interface {
	Get(ctx context.Context, id string) (*eventdestinations.EventDestination, error)
	Create(ctx context.Context, input eventdestinations.CreateInput) (*eventdestinations.EventDestination, error)
	Update(ctx context.Context, id string, input eventdestinations.UpdateInput) (*eventdestinations.EventDestination, error)
	Delete(ctx context.Context, id string) (*eventdestinations.EventDestination, error)
	Pause(ctx context.Context, id string) (*eventdestinations.EventDestination, error)
	Resume(ctx context.Context, id string) (*eventdestinations.EventDestination, error)
}

var _ eventDestinationsAPI = (*eventdestinations.Service)(nil)

func resourceEventDestination() *schema.Resource {
	return &schema.Resource{
		Description: "A place outside Massdriver that receives the organization's audit log events. " +
			"See [event destinations](https://docs.massdriver.cloud/platform-operations/event-destinations).",

		CreateContext: resourceEventDestinationCreate,
		ReadContext:   resourceEventDestinationRead,
		UpdateContext: resourceEventDestinationUpdate,
		DeleteContext: resourceEventDestinationDelete,
		CustomizeDiff: resourceEventDestinationCustomizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Name of the destination, unique in the organization.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"adapter": {
				Description: "How the destination receives events. `HTTP` posts each event to a URL as a CloudEvent, signed with Standard Webhooks headers. Changing it replaces the destination.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"config": {
				Description:      "JSON-encoded settings of the adapter, such as `jsonencode({ url = \"https://hooks.example.com/massdriver\", headers = { \"X-Team\" = \"platform\" } })` for `HTTP`. The API returns it and it is stored in state, so put credentials in `auth`.",
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsJSON,
				DiffSuppressFunc: structure.SuppressJsonDiff,
			},
			"auth": {
				Description:      "JSON-encoded credentials of the adapter, such as `jsonencode({ headers = { Authorization = \"Bearer ...\" } })` for `HTTP`. Write-only: it is not stored in state, and the API never returns it. It is sent when the destination is created and when `auth_version` changes. Requires Terraform 1.11 or OpenTofu 1.11 or later.",
				Type:             schema.TypeString,
				Optional:         true,
				WriteOnly:        true,
				ValidateDiagFunc: validateAuthJSON,
			},
			"auth_version": {
				Description: "Change this value to send `auth` again, for example to rotate credentials. If `auth` is not set when it changes, the destination's credentials are removed.",
				Type:        schema.TypeInt,
				Optional:    true,
			},
			"event_types": {
				Description: "Event types the destination receives, or `<entity>.*` for every event of an entity. Empty receives every event. The `massdriver_event_types` data source lists the event types. A change applies to events recorded after it.",
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"attributes": {
				Description: "Attribute values the event's entity must have had, such as `{ \"md-environment\" = \"prod\" }`. An event must have all of them. Empty receives events of any attributes. A change applies to events recorded after it.",
				Type:        schema.TypeMap,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"condition": {
				Description: "A jq expression run on each CloudEvent that `event_types` and `attributes` select. The event is sent only when it outputs `true`.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"paused": {
				Description: "Whether the destination is paused. Events wait while it is paused, and are sent when it is resumed. Massdriver pauses a destination when its sends fail for 24 hours (by default); the next plan then shows `paused` changing back to `false`, and apply resumes it.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
			"status": {
				Description: "`ACTIVE` or `PAUSED`.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"signing_secret": {
				Description: "The secret that signs each request, in Standard Webhooks form (`whsec_` followed by base64). Use it to check the `webhook-signature` header. Empty for adapters that do not sign.",
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
			},
			"failing_since": {
				Description: "When sends to the destination started to fail (RFC 3339). Empty when the last send succeeded.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"last_error": {
				Description: "Why the endpoint did not accept the last send, such as `HTTP 503`. Empty when the last send succeeded.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"last_delivered_at": {
				Description: "When the destination last accepted an event (RFC 3339).",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceEventDestinationCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	auth, diags := eventDestinationAuth(d)
	if diags.HasError() {
		return diags
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(d.Get("config").(string)), &config); err != nil {
		return diag.Errorf("config must be a JSON object: %s", err)
	}

	destination, err := pc.EventDestinations.Create(ctx, eventdestinations.CreateInput{
		Name:       d.Get("name").(string),
		Adapter:    d.Get("adapter").(string),
		Config:     config,
		Auth:       auth,
		EventTypes: expandStringList(d.Get("event_types").(*schema.Set).List()),
		Attributes: expandStringMap(d.Get("attributes").(map[string]any)),
		Condition:  d.Get("condition").(string),
	})
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(destination.ID)

	if d.Get("paused").(bool) {
		if destination, err = pc.EventDestinations.Pause(ctx, destination.ID); err != nil {
			return diag.FromErr(err)
		}
	}
	return setEventDestination(d, destination)
}

func resourceEventDestinationRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	destination, err := pc.EventDestinations.Get(ctx, d.Id())
	if err != nil {
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	return setEventDestination(d, destination)
}

func resourceEventDestinationUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	input := eventdestinations.UpdateInput{}
	changed := false
	if d.HasChange("name") {
		name := d.Get("name").(string)
		input.Name = &name
		changed = true
	}
	if d.HasChange("config") {
		if err := json.Unmarshal([]byte(d.Get("config").(string)), &input.Config); err != nil {
			return diag.Errorf("config must be a JSON object: %s", err)
		}
		changed = true
	}
	if d.HasChange("auth_version") {
		auth, diags := eventDestinationAuth(d)
		if diags.HasError() {
			return diags
		}
		// An empty auth removes the credentials; nil would leave them.
		if auth == nil {
			auth = map[string]any{}
		}
		input.Auth = auth
		changed = true
	}
	if d.HasChange("event_types") {
		eventTypes := expandStringList(d.Get("event_types").(*schema.Set).List())
		input.EventTypes = &eventTypes
		changed = true
	}
	if d.HasChange("attributes") {
		input.Attributes = expandStringMap(d.Get("attributes").(map[string]any))
		changed = true
	}
	if d.HasChange("condition") {
		condition := d.Get("condition").(string)
		input.Condition = &condition
		changed = true
	}

	var destination *eventdestinations.EventDestination
	var err error
	if changed {
		if destination, err = pc.EventDestinations.Update(ctx, d.Id(), input); err != nil {
			return diag.FromErr(err)
		}
	}
	if d.HasChange("paused") {
		if d.Get("paused").(bool) {
			destination, err = pc.EventDestinations.Pause(ctx, d.Id())
		} else {
			destination, err = pc.EventDestinations.Resume(ctx, d.Id())
		}
		if err != nil {
			return diag.FromErr(err)
		}
	}
	return setEventDestination(d, destination)
}

func resourceEventDestinationDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	if _, err := pc.EventDestinations.Delete(ctx, d.Id()); err != nil && !errors.Is(err, gql.ErrNotFound) {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

// resourceEventDestinationCustomizeDiff plans status and failing_since as
// unknown when paused changes, since pausing and resuming change them.
func resourceEventDestinationCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	if d.Id() == "" || !d.HasChange("paused") {
		return nil
	}
	if err := d.SetNewComputed("status"); err != nil {
		return err
	}
	return d.SetNewComputed("failing_since")
}

// eventDestinationAuth reads the write-only auth from the configuration,
// since write-only values are never in the plan or state. It returns nil
// when auth is not set.
func eventDestinationAuth(d *schema.ResourceData) (map[string]any, diag.Diagnostics) {
	raw, diags := d.GetRawConfigAt(cty.GetAttrPath("auth"))
	if diags.HasError() {
		return nil, diags
	}
	if raw.IsNull() {
		return nil, nil
	}
	auth, err := parseAuthJSON(raw.AsString())
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return auth, nil
}

// The JSON error is not wrapped, because it can quote the credentials.
func parseAuthJSON(s string) (map[string]any, error) {
	var auth map[string]any
	if err := json.Unmarshal([]byte(s), &auth); err != nil || auth == nil {
		return nil, errors.New("auth must be a JSON object")
	}
	return auth, nil
}

func validateAuthJSON(v any, path cty.Path) diag.Diagnostics {
	if _, err := parseAuthJSON(v.(string)); err != nil {
		return diag.Diagnostics{{Severity: diag.Error, Summary: err.Error(), AttributePath: path}}
	}
	return nil
}

func setEventDestination(d *schema.ResourceData, destination *eventdestinations.EventDestination) diag.Diagnostics {
	config, err := json.Marshal(destination.Config)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("name", destination.Name)
	d.Set("adapter", destination.Adapter)
	d.Set("config", string(config))
	d.Set("event_types", destination.EventTypes)
	d.Set("attributes", destination.Attributes)
	d.Set("condition", destination.Condition)
	d.Set("paused", destination.Status == string(eventdestinations.StatusPaused))
	d.Set("status", destination.Status)
	d.Set("signing_secret", destination.SigningSecret)
	d.Set("failing_since", formatOptionalTime(destination.FailingSince))
	d.Set("last_error", destination.LastError)
	d.Set("last_delivered_at", formatOptionalTime(destination.LastDeliveredAt))
	return nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func expandStringMap(raw map[string]any) map[string]string {
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		s, _ := v.(string)
		out[k] = s
	}
	return out
}
