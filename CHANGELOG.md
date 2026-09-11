# Changelog

## 1.4.1

A **compatibility release for v1 users**, backported onto the v1.4 line after
v2.2.0. It contains no new features and no breaking changes — upgrade within
v1 is safe and requires no bundle changes.

It does two things, both aimed at keeping v1 bundles deploying against a
current Massdriver CLI:

- The CLI no longer emits `schema-artifacts.json`. Every v1 provider (v1.0.0
  through v1.4.0) read that file from disk and hard-failed the apply when it
  was missing, so a bundle built with a current CLI but pinned to a v1 provider
  would break on deploy and have to be republished. That failure mode is gone.
- `massdriver.yaml`'s modern `resources` block is now understood, alongside the
  legacy `artifacts` block, so a bundle can adopt the new spec format without
  first migrating to the v2 provider.

If your bundle's `required_providers` uses a constraint that allows 1.4.1
(`~> 1.0`, `~> 1.4`, or no constraint), a `terraform init` picks this up with
no config change. v2.2.0 remains the latest release; this does not displace it.

### Added

- **Support for the modern `resources` block in `massdriver.yaml`.** The type
  for a field is now read from `resources.<field>.resource_type`, falling back
  to the legacy `artifacts.properties.<field>.$ref` when the field isn't in
  `resources`. Both `massdriver_artifact` and `massdriver_resource` accept
  either layout, so a bundle can move to the new spec format without leaving
  the v1 provider:

  ```yaml
  # modern
  resources:
    your_first_artifact:
      resource_type: your-org/getting-started-resource
      required: true

  # legacy — still fully supported
  artifacts:
    required:
      - your_first_artifact
    properties:
      your_first_artifact:
        $ref: getting-started-resource
  ```

  When a field appears in both blocks, `resources` wins. As before, a bare type
  ID is prefixed with your org ID and an already-qualified `org/type` is passed
  through unchanged — v1 sends org-qualified types on the wire, which is
  unchanged by this release.

### Fixed

- **A missing or unusable `schema-artifacts.json` no longer fails the apply.**
  Client-side JSON Schema validation is now skipped — and validation left to
  the server, which has been the authority since v2 — when the schema file is
  absent, unparseable, or has no schema for the resource's `field`. Affects
  `massdriver_artifact` (on both create and update) and `massdriver_resource`.
  A schema that *is* present and resolvable is still enforced exactly as
  before.

- **A non-object schema no longer panics the provider.** A `field` whose schema
  was a JSON boolean (`{"properties": {"vpc": true}}` — legal JSON Schema) hit
  an unchecked type assertion and crashed with
  `interface conversion: interface {} is bool, not map[string]interface{}`.
  It is now handled as an unenforceable schema and skipped.

### Notes

- `schema_path` is unchanged and still accepted, so no existing configuration
  or state needs to be touched. It is now a no-op when the file it points at
  is absent.
- `massdriver.yaml` is still required — it is read to resolve the resource type
  and is unaffected by this change.

## 1.3.0

v1.3.0 is a **bridge release**. The two new resources (`massdriver_resource`,
`massdriver_instance_alarm`) land alongside the two existing ones
(`massdriver_artifact`, `massdriver_package_alarm`), which are now deprecated
but remain fully functional. v2.0 removes the deprecated resources entirely.

You should migrate your bundles to the new resources at your own pace before
upgrading to v2.0.

### Added

- **`massdriver_resource`** — replaces `massdriver_artifact`. Uses the same REST
  endpoint (`/v1/artifacts` redirects to `/v1/resources` server-side). Only
  usable inside a Massdriver bundle deployment; fast-fails with a clear error
  when run with non-deployment credentials.

- **`massdriver_instance_alarm`** — replaces `massdriver_package_alarm`. Backed
  by GraphQL, so it works inside *or* outside a bundle deployment.
  `instance_id` defaults from `MASSDRIVER_INSTANCE_ID` if set, otherwise from
  `MASSDRIVER_PACKAGE_NAME` with the trailing deployment suffix stripped — so
  bundle deployments get the right ID for free.

### Changed

- **`massdriver_package_alarm`** internals now go through the GraphQL
  `instance_alarm` endpoint. The REST endpoint that previously backed this
  resource has been removed from the server. There is **no schema change**;
  existing configs keep working without modification.

  Create now performs a **self-heal lookup**: if an alarm with the same
  `cloud_resource_id` already exists on the instance, it's adopted into state
  rather than duplicated. This recovers state files corrupted by pre-1.3
  deploys, which 404'd against the removed REST endpoint and silently cleared
  the alarm's UUID from state.

- **`massdriver_artifact`** is marked deprecated via `DeprecationMessage`.
  Create/Read/Update/Delete remain fully functional. Will be removed in v2.0.

- **`massdriver_package_alarm`** is marked deprecated via `DeprecationMessage`.
  Will be removed in v2.0.

### Migration Guide

Do **not** manage the same record via both the old and new resources
simultaneously — terraform won't detect the conflict and the two will fight
over state. Migrate by renaming.

#### `massdriver_artifact` → `massdriver_resource`

```hcl
# Before (v1.x)                                # After (v1.3+)
resource "massdriver_artifact" "vpc" {         resource "massdriver_resource" "vpc" {
  field    = "vpc"                               field    = "vpc"
  name     = "My VPC"                            name     = "My VPC"
  artifact = jsonencode({...})                   resource = jsonencode({...})
}                                              }
```

- Rename the resource type from `massdriver_artifact` to `massdriver_resource`.
- Rename the `artifact` argument to `resource`.
- Drop `provider_resource_id`, `type`, and `last_updated` — they no longer
  exist.
- `resource_type` is now `Computed` from `massdriver.yaml`'s `$ref`; don't set
  it explicitly.

After renaming in HCL, use `terraform state mv` (or a `moved {}` block) to
preserve state without recreating the record server-side:

```hcl
moved {
  from = massdriver_artifact.vpc
  to   = massdriver_resource.vpc
}
```

#### `massdriver_package_alarm` → `massdriver_instance_alarm`

```hcl
# Before (v1.x)                                       # After (v1.3+)
resource "massdriver_package_alarm" "high_cpu" {      resource "massdriver_instance_alarm" "high_cpu" {
  package_id        = "..."                             # instance_id defaults from env in bundles;
  cloud_resource_id = "..."                             # set explicitly outside deployments.
  display_name      = "..."                             cloud_resource_id = "..."
  period_minutes    = 5                                 display_name      = "..."
  threshold         = 80                                period            = 300   # SECONDS, not minutes
  comparison_operator = "GreaterThanThreshold"          threshold         = 80
  metric {                                              comparison_operator = "GreaterThanThreshold"
    name = "..."                                        metric {
    namespace = "..."                                     name = "..."
    statistic = "Average"                                 namespace = "..."
    dimensions = { ... }                                  statistic = "Average"
  }                                                       dimensions = { ... }
}                                                       }
                                                      }
```

- Rename the resource type from `massdriver_package_alarm` to
  `massdriver_instance_alarm`.
- Rename `package_id` → `instance_id`, *or* omit it entirely and let the
  default pick it up from `MASSDRIVER_INSTANCE_ID` / `MASSDRIVER_PACKAGE_NAME`.
- Rename `period_minutes` → `period` and **multiply the value by 60**
  (`period` is seconds, not minutes).
- `last_updated` is no longer surfaced — drop it.
- `metric.region` is now available (Optional) for cases where the cloud
  provider exposes per-region metrics.

Same `moved {}` pattern preserves state:

```hcl
moved {
  from = massdriver_package_alarm.high_cpu
  to   = massdriver_instance_alarm.high_cpu
}
```

### Fixed

- `massdriver_package_alarm` Read no longer hits the (now-removed) REST
  endpoint. Refresh against existing state works again under v1.3.

- Pre-1.3 deploys that 404'd against the removed REST endpoint and silently
  cleared an alarm's UUID from state are now self-healed by the new
  Create path's lookup-by-`cloud_resource_id`.
