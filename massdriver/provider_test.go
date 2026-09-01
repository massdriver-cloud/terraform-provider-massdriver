package massdriver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/massdriver-cloud/massdriver-sdk-go/massdriver/config"
)

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

// crudOf returns r's non-nil CRUD entry points by name.
func crudOf(r *schema.Resource) map[string]func(context.Context, *schema.ResourceData, any) diag.Diagnostics {
	all := map[string]func(context.Context, *schema.ResourceData, any) diag.Diagnostics{
		"create": r.CreateContext,
		"read":   r.ReadContext,
		"update": r.UpdateContext,
		"delete": r.DeleteContext,
	}
	for op, fn := range all {
		if fn == nil {
			delete(all, op)
		}
	}
	return all
}

// Every resource must be in exactly one auth bucket, so a new one can't end
// up unguarded or registered twice.
func TestEveryResourceIsClassified(t *testing.T) {
	seen := map[string]int{}
	for _, bucket := range []map[string]*schema.Resource{
		apiKeyResources(), anyCredentialResources(), deploymentTokenResources(),
	} {
		for name := range bucket {
			seen[name]++
		}
	}

	for name := range Provider().ResourcesMap {
		switch seen[name] {
		case 1:
		case 0:
			t.Errorf("%s is in no auth bucket", name)
		default:
			t.Errorf("%s is in %d auth buckets, want 1", name, seen[name])
		}
	}
	if len(seen) != len(Provider().ResourcesMap) {
		t.Errorf("buckets classify %d resources, provider registers %d", len(seen), len(Provider().ResourcesMap))
	}
}

// API-key resources report the missing key rather than dereferencing a nil
// service. Walks the real bucket, so new resources are covered automatically.
func TestAPIKeyResourcesGuarded(t *testing.T) {
	pc := &ProviderClient{PlatformAuthErr: config.ErrNoCredentials}

	for name := range apiKeyResources() {
		t.Run(name, func(t *testing.T) {
			r := Provider().ResourcesMap[name]
			for op, fn := range crudOf(r) {
				diags := fn(context.Background(), r.TestResourceData(), pc)
				if !diags.HasError() {
					t.Errorf("%s: got no error, want the missing-API-key diagnostic", op)
					continue
				}
				if !strings.Contains(diags[0].Summary, "No Massdriver API key") {
					t.Errorf("%s: summary = %q, want the missing-API-key diagnostic", op, diags[0].Summary)
				}
			}
		})
	}
}

// Regression: bundles pair massdriver_instance_alarm with massdriver_resource
// under the deployment token alone. A missing API key must not block them.
func TestInstanceAlarmRunsWithDeploymentTokenOnly(t *testing.T) {
	fake := &fakeInstanceAlarms{getErr: errors.New("sentinel: reached the resource body")}
	// What NewProviderClient produces with a deployment token and no API key.
	pc := &ProviderClient{
		PlatformAuthErr: config.ErrNoCredentials,
		InstanceAlarms:  fake,
	}

	r := Provider().ResourcesMap["massdriver_instance_alarm"]
	d := r.TestResourceData()
	d.SetId("alarm-1")

	diags := r.ReadContext(context.Background(), d, pc)
	if !diags.HasError() || !strings.Contains(diags[0].Summary, "sentinel") {
		t.Fatalf("diags = %v, want the resource body to run despite PlatformAuthErr", diags)
	}
}

// With neither credential, alarms report that rather than panicking.
func TestAnyCredentialResourcesGuardedWithNoCredentials(t *testing.T) {
	pc := &ProviderClient{
		PlatformAuthErr: config.ErrNoCredentials,
		AlarmAuthErr:    config.ErrDeploymentCredentialsMissing,
	}

	for name := range anyCredentialResources() {
		t.Run(name, func(t *testing.T) {
			r := Provider().ResourcesMap[name]
			for op, fn := range crudOf(r) {
				diags := fn(context.Background(), r.TestResourceData(), pc)
				if !diags.HasError() {
					t.Errorf("%s: got no error, want the missing-credentials diagnostic", op)
					continue
				}
				if !strings.Contains(diags[0].Summary, "No Massdriver credentials") {
					t.Errorf("%s: summary = %q, want the missing-credentials diagnostic", op, diags[0].Summary)
				}
			}
		})
	}
}

// massdriver_resource is never guarded: it has no API-key path at all, so a
// guard could only ever suggest a credential that would not help. It resolves
// its own client and reports its own error.
func TestDeploymentTokenResourceNotGuarded(t *testing.T) {
	for name := range deploymentTokenResources() {
		pc := &ProviderClient{
			PlatformAuthErr: config.ErrNoCredentials,
			AlarmAuthErr:    config.ErrDeploymentCredentialsMissing,
			ProvisioningResources: func() (provisioningResourcesAPI, error) {
				return nil, errors.New("sentinel: reached the resource body")
			},
		}
		r := Provider().ResourcesMap[name]
		diags := r.ReadContext(context.Background(), r.TestResourceData(), pc)
		if !diags.HasError() || !strings.Contains(diags[0].Summary, "sentinel") {
			t.Errorf("%s: diags = %v, want the resource body to run", name, diags)
		}
	}
}
