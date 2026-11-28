package massdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/gql"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/platform/stash"
)

// stashAPI is the slice of *stash.Service that the stash resource and data
// sources call.
type stashAPI interface {
	Get(ctx context.Context, scope stash.Scope, key string) (*stash.Entry, error)
	Iter(ctx context.Context, scope stash.Scope, input stash.ListInput) iter.Seq2[stash.Entry, error]
	Set(ctx context.Context, scope stash.Scope, key string, input stash.SetInput) (*stash.Entry, error)
	Delete(ctx context.Context, scope stash.Scope, key string) error
}

var _ stashAPI = (*stash.Service)(nil)

const stashAlphaNotice = "Alpha: this resource can change or be removed at any time."

// stashScopeKinds is the order the scope attributes are checked in. Each kind
// is also the name of its schema attribute.
var stashScopeKinds = []stash.ScopeKind{
	stash.ScopeProject,
	stash.ScopeEnvironment,
	stash.ScopeComponent,
	stash.ScopeInstance,
	stash.ScopeDeployment,
}

var stashKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,255}$`)

// stashMaxTTL is ten years in seconds, the server's upper bound.
const stashMaxTTL = 315360000

// stashWritableFields are the attributes a set writes. A set replaces the
// whole entry, so a change to any of them is one set call.
var stashWritableFields = []string{"value", "sensitive_value", "format", "label", "ttl", "immutable"}

func stashScopeNames() []string {
	names := make([]string, len(stashScopeKinds))
	for i, k := range stashScopeKinds {
		names[i] = string(k)
	}
	return names
}

func stashScopeDescriptions() map[stash.ScopeKind]string {
	return map[stash.ScopeKind]string{
		stash.ScopeProject:     "Identifier of the project whose stash holds the entry, such as `ecomm`.",
		stash.ScopeEnvironment: "Identifier of the environment whose stash holds the entry, such as `ecomm-prod`.",
		stash.ScopeComponent:   "Identifier of the component whose stash holds the entry, such as `ecomm-db`.",
		stash.ScopeInstance:    "Identifier of the instance whose stash holds the entry, such as `ecomm-prod-db`.",
		stash.ScopeDeployment:  "UUID of the deployment whose stash holds the entry. Inside a bundle deployment, this is the `MASSDRIVER_DEPLOYMENT_ID` environment variable.",
	}
}

// stashScopeSchema returns the five scope attributes. Exactly one is set.
func stashScopeSchema(forceNew bool) map[string]*schema.Schema {
	names := stashScopeNames()
	out := make(map[string]*schema.Schema, len(names))
	for kind, desc := range stashScopeDescriptions() {
		out[string(kind)] = &schema.Schema{
			Description:  desc + " Set exactly one of `project`, `environment`, `component`, `instance`, or `deployment`.",
			Type:         schema.TypeString,
			Optional:     true,
			ForceNew:     forceNew,
			ExactlyOneOf: names,
			ValidateFunc: validation.StringIsNotEmpty,
		}
	}
	return out
}

func stashKeySchema(forceNew bool) *schema.Schema {
	return &schema.Schema{
		Description:  "The entry's key. Letters, digits, and the characters `_`, `.`, `/`, and `-`, up to 255 characters. Keys that start with `md/` are well-known keys. The server checks their values against a schema.",
		Type:         schema.TypeString,
		Required:     true,
		ForceNew:     forceNew,
		ValidateFunc: validation.StringMatch(stashKeyPattern, "must be 1-255 characters: letters, digits, `_`, `.`, `/`, or `-`"),
	}
}

func resourceStashEntry() *schema.Resource {
	fields := map[string]*schema.Schema{
		"key": stashKeySchema(true),
		"value": {
			Description:      "The entry's value. Use `jsonencode` for a `JSON` entry. Set exactly one of `value` or `sensitive_value`. A `value` shows in plan output.",
			Type:             schema.TypeString,
			Optional:         true,
			ExactlyOneOf:     []string{"value", "sensitive_value"},
			DiffSuppressFunc: suppressEquivalentStashValue,
		},
		"sensitive_value": {
			Description:      "The entry's value, stored as a sensitive entry. Massdriver encrypts it. Only a caller with `stash:export`, or a deployment token, can read it. Terraform hides it in plan output. Set exactly one of `value` or `sensitive_value`. A key that starts with `md/` cannot be sensitive.",
			Type:             schema.TypeString,
			Optional:         true,
			Sensitive:        true,
			ExactlyOneOf:     []string{"value", "sensitive_value"},
			ValidateFunc:     validation.StringIsNotEmpty,
			DiffSuppressFunc: suppressEquivalentStashValue,
		},
		"format": {
			Description:  "How the value is read: `TEXT` or `JSON`. A `JSON` value must be a JSON object. Defaults to `TEXT`.",
			Type:         schema.TypeString,
			Optional:     true,
			Default:      string(stash.FormatText),
			ValidateFunc: validation.StringInSlice([]string{string(stash.FormatText), string(stash.FormatJSON)}, false),
		},
		"label": {
			Description:  "A name for the entry, up to 255 characters. An entry with a label shows as a row on the Metadata panels. A label is not encrypted, so do not put secrets in it.",
			Type:         schema.TypeString,
			Optional:     true,
			ValidateFunc: validation.StringLenBetween(0, 255),
		},
		"ttl": {
			Description:  "Seconds until the entry expires, up to 315360000 (10 years). `0` keeps the entry until it is deleted. Each write starts the time again. The API does not return the TTL, so a TTL changed outside Terraform does not show as a change. After the entry expires, the next plan shows it as missing and the next apply writes it again. Do not use Terraform to manage a value that must expire and stay gone.",
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      0,
			ValidateFunc: validation.IntBetween(0, stashMaxTTL),
		},
		"immutable": {
			Description: "When `true`, the server refuses a second write to the key. A change to any argument of an immutable entry deletes the entry and writes it again. Defaults to `false`.",
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
		},
		"sensitive": {
			Description: "Whether the entry is sensitive. It is `true` when `sensitive_value` is set.",
			Type:        schema.TypeBool,
			Computed:    true,
		},
	}
	for name, s := range stashScopeSchema(true) {
		fields[name] = s
	}

	return &schema.Resource{
		Description: stashAlphaNotice + "\n\n" +
			"An entry in the Massdriver stash of a project, environment, component, instance, or deployment. " +
			"A write replaces the whole entry. Creating this resource overwrites an entry that already has the key. " +
			"Changing the scope or the key replaces the entry. Changing an immutable entry deletes it and writes it again. " +
			"Do not use `create_before_destroy` with this resource: the old and new entries have the same key, so deleting the old entry deletes the new one.",

		CreateContext: resourceStashEntryCreate,
		ReadContext:   resourceStashEntryRead,
		UpdateContext: resourceStashEntryUpdate,
		DeleteContext: resourceStashEntryDelete,
		CustomizeDiff: resourceStashEntryCustomizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: resourceStashEntryImport,
		},

		Schema: fields,
	}
}

func resourceStashEntryCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	entry, diags := writeStashEntry(ctx, d, meta)
	if diags.HasError() {
		return diags
	}
	scope, _ := stashScopeFromData(d)
	d.SetId(stashEntryID(scope, d.Get("key").(string)))
	setStashEntryState(d, entry)
	return nil
}

func resourceStashEntryUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	entry, diags := writeStashEntry(ctx, d, meta)
	if diags.HasError() {
		return diags
	}
	setStashEntryState(d, entry)
	return nil
}

// writeStashEntry sets the entry and returns what the server stored. State
// comes from that result, not from a read: an entry with a short TTL can
// expire before a read.
func writeStashEntry(ctx context.Context, d *schema.ResourceData, meta any) (*stash.Entry, diag.Diagnostics) {
	pc := meta.(*ProviderClient)

	scope, err := stashScopeFromData(d)
	if err != nil {
		return nil, diag.FromErr(err)
	}

	input := stash.SetInput{
		Label:     d.Get("label").(string),
		Format:    stash.Format(d.Get("format").(string)),
		TTL:       time.Duration(d.Get("ttl").(int)) * time.Second,
		Immutable: d.Get("immutable").(bool),
	}
	if v := d.Get("sensitive_value").(string); v != "" {
		input.Value = v
		input.Sensitive = true
	} else {
		input.Value = d.Get("value").(string)
	}

	entry, err := pc.Stash.Set(ctx, scope, d.Get("key").(string), input)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return entry, nil
}

func resourceStashEntryRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	scope, err := stashScopeFromData(d)
	if err != nil {
		return diag.FromErr(err)
	}

	entry, err := pc.Stash.Get(ctx, scope, d.Get("key").(string))
	if err != nil {
		// Deleted or expired: clear state so the next plan writes it again.
		if errors.Is(err, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	setStashEntryState(d, entry)
	return nil
}

func setStashEntryState(d *schema.ResourceData, entry *stash.Entry) {
	d.Set("format", entry.Format)
	d.Set("label", entry.Label)
	d.Set("immutable", entry.Immutable)
	d.Set("sensitive", entry.Sensitive)

	// A nil value is a sensitive value the caller cannot read. State keeps
	// the configured value so the hidden value does not show as a change.
	if entry.Value != nil {
		if entry.Sensitive {
			d.Set("sensitive_value", *entry.Value)
			d.Set("value", nil)
		} else {
			d.Set("value", *entry.Value)
			d.Set("sensitive_value", nil)
		}
	}
}

func resourceStashEntryDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	pc := meta.(*ProviderClient)

	scope, err := stashScopeFromData(d)
	if err != nil {
		return diag.FromErr(err)
	}
	key := d.Get("key").(string)

	if err := pc.Stash.Delete(ctx, scope, key); err != nil {
		// The server reports a missing entry as a failed mutation, not as
		// not found. An entry that is already gone does not block destroy.
		_, getErr := pc.Stash.Get(ctx, scope, key)
		if errors.Is(getErr, gql.ErrNotFound) {
			d.SetId("")
			return nil
		}
		if getErr != nil {
			return diag.FromErr(fmt.Errorf("%w; reading the entry after the failed delete also failed: %w", err, getErr))
		}
		return diag.FromErr(err)
	}

	d.SetId("")
	return nil
}

func resourceStashEntryImport(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
	scope, key, err := parseStashEntryID(d.Id())
	if err != nil {
		return nil, err
	}
	d.Set(string(scope.Kind), scope.ID)
	d.Set("key", key)
	d.SetId(stashEntryID(scope, key))
	return []*schema.ResourceData{d}, nil
}

func resourceStashEntryCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	if err := validateStashJSONValue(d); err != nil {
		return err
	}

	if d.Id() != "" {
		// The server refuses a write to an immutable entry, so a change
		// deletes the entry and writes it again.
		if wasImmutable, _ := d.GetChange("immutable"); wasImmutable.(bool) {
			for _, k := range stashWritableFields {
				if d.HasChange(k) {
					if err := d.ForceNew(k); err != nil {
						return err
					}
				}
			}
		}
	}

	// Read from raw config: sensitive_value can be unknown at plan time, but
	// whether it is set is known.
	sensitive := d.Get("sensitive_value").(string) != ""
	if raw := d.GetRawConfig(); !raw.IsNull() && raw.Type().IsObjectType() {
		sensitive = !raw.GetAttr("sensitive_value").IsNull()
	}
	if d.Get("sensitive").(bool) != sensitive {
		return d.SetNew("sensitive", sensitive)
	}
	return nil
}

// validateStashJSONValue fails the plan when a JSON entry's value is known and
// is not a JSON object, which the server rejects.
func validateStashJSONValue(d *schema.ResourceDiff) error {
	if d.Get("format").(string) != string(stash.FormatJSON) {
		return nil
	}
	for _, k := range []string{"value", "sensitive_value"} {
		if !d.NewValueKnown(k) {
			continue
		}
		v := d.Get(k).(string)
		if v == "" {
			continue
		}
		if _, ok := parseJSONObject(v); !ok {
			return fmt.Errorf("%s must be a JSON object when format is JSON; use jsonencode", k)
		}
	}
	return nil
}

// suppressEquivalentStashValue hides a change between two JSON values that
// differ only in key order or whitespace.
func suppressEquivalentStashValue(_, old, new string, d *schema.ResourceData) bool {
	if d.Get("format").(string) != string(stash.FormatJSON) {
		return false
	}
	return jsonEquivalent(old, new)
}

// jsonEquivalent reports whether a and b are both JSON objects with the same
// content.
func jsonEquivalent(a, b string) bool {
	av, ok := parseJSONObject(a)
	if !ok {
		return false
	}
	bv, ok := parseJSONObject(b)
	if !ok {
		return false
	}
	an, _ := json.Marshal(av)
	bn, _ := json.Marshal(bv)
	return bytes.Equal(an, bn)
}

// parseJSONObject decodes s when it is exactly one JSON object. Numbers stay
// json.Number so large integers keep their digits.
func parseJSONObject(s string) (map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	return obj, true
}

// stashScopeFromData returns the one scope attribute that is set. Schema
// validation guarantees exactly one in configuration.
func stashScopeFromData(d interface{ Get(string) any }) (stash.Scope, error) {
	for _, kind := range stashScopeKinds {
		if id, _ := d.Get(string(kind)).(string); id != "" {
			return stash.Scope{Kind: kind, ID: id}, nil
		}
	}
	return stash.Scope{}, errors.New("exactly one of project, environment, component, instance, or deployment must be set")
}

// stashEntryID is `<kind>:<scope-id>/<key>`.
func stashEntryID(scope stash.Scope, key string) string {
	return scope.String() + "/" + key
}

// parseStashEntryID reads `<kind>:<scope-id>/<key>`. Scope IDs hold no `/`,
// so everything after the first `/` is the key, which can hold `/`.
func parseStashEntryID(id string) (stash.Scope, string, error) {
	invalid := fmt.Errorf("invalid stash entry ID %q: expected <kind>:<scope-id>/<key>, where kind is one of %s", id, strings.Join(stashScopeNames(), ", "))

	kind, rest, ok := strings.Cut(id, ":")
	if !ok {
		return stash.Scope{}, "", invalid
	}
	scopeID, key, ok := strings.Cut(rest, "/")
	if !ok || scopeID == "" || !stashKeyPattern.MatchString(key) {
		return stash.Scope{}, "", invalid
	}
	for _, k := range stashScopeKinds {
		if string(k) == kind {
			return stash.Scope{Kind: k, ID: scopeID}, key, nil
		}
	}
	return stash.Scope{}, "", invalid
}
