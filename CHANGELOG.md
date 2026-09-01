# Changelog

## 2.2.0

API key and deployment token auth now work side by side. Each API surface
authenticates with the credential it requires, instead of a deployment token
in the environment shadowing an API key.

### Upgrade notes

- **`decommission_protection` is sent on every apply, so an environment
  protected out of band will be unprotected.** If someone enabled
  decommission protection in the console and the environment's terraform
  config doesn't set `decommission_protection = true`, the next apply turns
  it off — the field defaults to `false` and omitting it means `false`, not
  "leave whatever is set". Before upgrading, add
  `decommission_protection = true` to any environment that should keep it.
  `separation_of_duty` behaves the same way.

### Fixed

- **An API key is no longer ignored when a deployment token is present.**
  Each resource authenticates with the credential its class requires:
  platform resources take an API key or PAT, and provisioning resources
  (`massdriver_resource`, `massdriver_instance_alarm`) take the
  `MASSDRIVER_DEPLOYMENT_ID` + `MASSDRIVER_TOKEN` pair the platform injects
  into a bundle deployment. Previously the SDK's resolver preferred the
  deployment token whenever both were set — even when the API key was set
  explicitly in the provider block — so platform resources failed with
  permission errors inside a bundle deployment with no way to opt out.
- **Using a platform resource without an API key now says so.** It fails
  at plan time with a diagnostic naming the resource and how to supply
  credentials, instead of attempting the call with a deployment token and
  returning a server-side permission error. Neither credential is required
  to configure the provider, so a bundle carrying only a deployment token —
  the usual case — is unaffected: provisioning resources work with no API
  key present.
- **`url` set in the provider block now reaches `massdriver_resource`.** It
  was previously read from `MASSDRIVER_URL` only for that resource.
  `organization_id` still comes from the environment there.

### Added

- **`separation_of_duty` and `decommission_protection` on
  `massdriver_environment`.** `separation_of_duty` requires a deployment
  proposal to be approved by someone other than the proposer;
  `decommission_protection` blocks decommissioning the environment and its
  instances. Both are optional and default to `false`. Neither is
  `Computed`: like `attributes`, omitting one means `false` rather than
  "leave whatever is set", so an out-of-band change in the console is
  reverted on the next apply. Note that turning on
  `decommission_protection` makes `terraform destroy` fail until it is
  turned back off.

### Changed

- **`attributes` is now optional** on `massdriver_project`,
  `massdriver_environment`, `massdriver_component`, and
  `massdriver_oci_repository`. Omitting it means "no attributes", exactly
  as `attributes = {}` did. Drift behavior is unchanged — attributes drive
  permissions, so console edits are still reverted on the next apply, and
  that now includes attributes added out of band to a resource whose
  config omits the field.

### Removed

- **Unprefixed environment variables are no longer honored.** Via the SDK
  bump, bare `API_KEY`, `TOKEN`, `URL`, `ORG_ID`, `ORGANIZATION_ID`,
  `PROFILE`, `DEPLOYMENT_ID`, and the `BUNDLE_*` / `INSTANCE_ID` /
  `STEP_PATH` names are ignored; only the `MASSDRIVER_`-prefixed forms are
  read. They were silently accepted as fallbacks before, so a generic
  `TOKEN` or `URL` in the environment could configure the provider by
  accident. If any pipeline relied on the unprefixed names, add the prefix.

### Internal

- Massdriver Go SDK bumped to v0.3.2, which makes deployment-token auth
  opt-in per client (`massdriver.WithDeploymentTokenAuth`) rather than an
  ambient default. The provider relies on that selection instead of
  resolving credentials itself.

## 2.1.1

`massdriver_resource` now treats the bundle's `massdriver.yaml` as the
single source of truth for a resource's type, resolved at plan time. This
adds support for versioned resource types: bumping a resource's version in
`massdriver.yaml` — with no other config change — now plans a replacement.
Previously a type change in the yaml was silently ignored until some other
attribute happened to change.

### Changed

- **`resource_type` is resolved at plan time** from
  `resources.<field>.resource_type` in `massdriver.yaml`, falling back to
  the legacy `artifacts.properties.<field>.$ref`. The value stored in
  state no longer takes precedence, so type changes in the yaml always
  surface in the plan. Because `resource_type` forces replacement, a
  version bump destroys and recreates the resource.
- **Resource types are canonically bare.** Org qualifiers
  (`<org>/aws-vpc`) are stripped from yaml values and from API responses;
  versions are preserved (`aws-vpc@2.0.0`). This also fixes a perpetual
  replacement diff caused by the API echoing org-qualified types back
  into state.

### Removed

- **`schema_path`** and the client-side JSON Schema validation of
  `resource` against `schema-artifacts.json`. Validation is handled
  server-side. The attribute was defaulted-only (never user-set) and its
  removal is state-safe — existing states are upgraded automatically. If
  you referenced `massdriver_resource.*.schema_path` in an expression,
  remove the reference.

### Internal

- Massdriver Go SDK bumped to v0.2.19; project, environment, and component
  updates migrated to its new partial-update (pointer-field) inputs. No
  user-facing change.

## 2.0.0

v2.0.0 is the **platform-management release**. The provider now manages
the Massdriver platform's first-class entities — projects, environments,
components, groups, policies, and resources — alongside the
deployment-side resources that have always been here.

Internally, the provider now uses the
[Massdriver Go SDK](https://github.com/massdriver-cloud/massdriver-sdk-go)
for every API call (GraphQL platform surface + deployment-token REST
surface), replacing the bundled genqlient client. No user-facing impact
beyond what's listed below.

### Breaking changes

- **Removed** `massdriver_artifact`. Use `massdriver_resource`. The v1.4
  bridge release introduced `massdriver_resource` specifically as the
  migration path — if you skipped v1.4, see the v1.4.X entry below for the
  side-by-side mapping.
- **Removed** `massdriver_package_alarm`. Use `massdriver_instance_alarm`.
  Same v1.4 migration path applies.

### Added

New top-level platform resources, all backed by GraphQL via the SDK:

- **`massdriver_project`** — top-level project (architecture blueprint
  container).
- **`massdriver_environment`** — deployment context within a project
  (prod, staging, etc.).
- **`massdriver_component`** — bundle slot in a project's blueprint,
  sourced from an OCI repository.
- **`massdriver_group`** — custom access-control group (built-in groups
  are platform-managed and not exposed here).
- **`massdriver_group_policy`** — ABAC policy attached to a group. Each
  policy grants (`ALLOW`) or blocks (`DENY`) one or more actions on
  resources matching the conditions; `DENY` wins.
- **`massdriver_imported_resource`** — register an existing cloud asset
  not managed by a Massdriver bundle, so other components can connect to
  it. Counterpart to `massdriver_resource` (which is for bundle-emitted
  resources).
- **`massdriver_oci_repository`** — manage repositories in the Massdriver
  OCI catalog. `artifact_type = "BUNDLE"` today; resource-type and
  provisioner repositories are planned and will be selectable via the
  same field.

`massdriver_resource` and `massdriver_instance_alarm` continue from v1.4,
now backed by the SDK rather than the in-tree genqlient client.

### Migration from v1.x

If you're upgrading from v1.0–v1.2 directly to v2.0, you must migrate off
`massdriver_artifact` and `massdriver_package_alarm` first. The cleanest
path is to bounce through v1.4 (which ships both old and new resources), 
 apply the migration, then upgrade to v2.0.

## 1.4.X

v1.4.X is a **bridge release**. The two new resources (`massdriver_resource`,
`massdriver_instance_alarm`) land alongside the two existing ones
(`massdriver_artifact`, `massdriver_package_alarm`), which are now deprecated
but remain fully functional. v2.0 removes the deprecated resources entirely.

You should migrate your bundles to the new resources at your own pace before
upgrading to v2.0.

### Added

- **`massdriver_resource`** — replaces `massdriver_artifact`. Only
  usable inside a Massdriver bundle deployment; fast-fails with a clear error
  when run with non-deployment credentials.

- **`massdriver_instance_alarm`** — replaces `massdriver_package_alarm`. Backed
  by GraphQL, so it works inside *or* outside a bundle deployment.

### Changed

- **`massdriver_package_alarm`** is marked deprecated via `DeprecationMessage`.
  Create/Read/Update/Delete remain fully functional. Will be removed in v2.0.

- **`massdriver_artifact`** is marked deprecated via `DeprecationMessage`.
  Create/Read/Update/Delete remain fully functional. Will be removed in v2.0.

### Migration Guide

Do **not** manage the same record via both the old and new resources
simultaneously — terraform won't detect the conflict and the two will fight
over state. Migrate by renaming.

#### `massdriver_artifact` → `massdriver_resource`

```hcl
# Before (v1.x)                                # After (v1.4+)
resource "massdriver_artifact" "vpc" {         resource "massdriver_resource" "vpc" {
  field    = "vpc"                               field    = "vpc"
  name     = "My VPC"                            name     = "My VPC"
  artifact = jsonencode({...})                   resource = jsonencode({...})
}                                              }
```

- Rename the resource type from `massdriver_artifact` to `massdriver_resource`.
- Rename the `artifact` argument to `resource`.
- Drop `provider_resource_id` and `type` — they no longer
  exist.

#### `massdriver_package_alarm` → `massdriver_instance_alarm`

```hcl
# Before (v1.x)                                       # After (v1.4+)
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
- Rename `period_minutes` → `period` and **multiply the value by 60**
  (`period` is seconds, not minutes).