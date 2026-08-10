# A config-server-backed `cache.Client` reads real Intents from the colocated `controller`'s cache instead of persisting a local copy

data-server gains a second `cache.Client` implementation, selected by `Cache.Type: config-server` in the existing `Server.createCacheClient` switch (`pkg/server/cache.go`), that serves real Intent documents by calling a new local unary Get/List surface on the config-server `controller` container colocated 1:1 with data-server in the StatefulSet — instead of persisting its own copy of intent content the way `LocalCache` does today. `"running"` (the synced device-state pseudo-intent) is unaffected by backend choice: it is split out onto its own accessor and always kept as a minimal in-memory, per-instance store, because it was never config-server's data to begin with.

`Cache.Type: local` keeps working completely unmodified — this is an additive backend, not a replacement, and every interface change below is a widening (new methods, or a return-type generalization that `LocalCache` already satisfies trivially) rather than a breaking one for the existing path.

## Why

Today `CacheClientBoundImpl` (`pkg/cache/cacheClientBound.go`) is a thin, backend-agnostic forwarder over a single package-wide `cache.Client`, and `LocalCache` (`pkg/cache/local.go`) is that interface's only implementation: a disk-backed store that treats every intent name — including `"running"` — identically. That homogeneity was a coincidence of `LocalCache` being the only backend, not a real unification: config-server is already the sole northbound writer of real Intents (`docs/architecture.md` §2), and is itself backed by a centralized, watch-synced store (a separate aggregated API server, not the colocated `controller` — see the research below). Persisting a second copy of that same data in `LocalCache` is redundant duplication with its own consistency problems; the colocated `controller`'s existing informer cache can serve reads locally instead.

`"running"` is structurally different: it is produced and consumed entirely in-process (`ops.TreeExport` / `treeproto.NewProtoTreeImporter`), nothing reloads it from a cache on startup, and `docs/architecture.md`'s cold-start story already puts recovery on the control plane rather than on durable local state. It has no reason to route through config-server at all, and no reason to be durable under this backend either.

## Scope note: this ADR documents a contract, not a config-server implementation

No local read API exists in config-server today. The realistic shape — unary Get-by-name / List-by-target, `localhost`-bound, backed by the `controller`'s existing watch-synced `client.Client`, scoped by the `TargetNamespaceKey`/`TargetNameKey` labels config-server's `Transactor.ListConfigsPerTarget` (`pkg/sdc/target/manager/transactor.go:500-518` in the config-server checkout) already uses — is genuinely cross-repo, net-new work in config-server. This ADR treats that contract as the assumed interface both sides implement against; it does not further design config-server's internals (no new auth posture beyond existing pod-level isolation, no new staleness handling beyond what the informer cache already accepts elsewhere in config-server).

**Baseline note:** this data-server checkout is itself on the unmerged `sensitive` branch (PR [#460](https://github.com/sdcio/data-server/pull/460)), paired with config-server's own unmerged `sensitive` branch (config-server#441). This ADR targets that paired branch pair as the realistic near-term baseline, not config-server's `main` — see the `SensitivePaths` row below, which depends on a CRD (`SensitiveConfig`) that exists on that branch only.

## Interface changes

**`importer.ImportConfigAdapter`** (`pkg/tree/importer/import_config_adapter.go`) gains two accessors, becoming the full "intent-shaped adapter" surface rather than a pure tree-traversal interface:

```go
type ImportConfigAdapter interface {
	ImportConfigAdapterElement
	GetDeletes() *sdcpb.PathSet
	GetName() string
	GetPriority() int32
	GetNonRevertive() bool
	GetOrphan() bool                   // new
	GetSensitivePaths() []*sdcpb.Path  // new
}
```

`ProtoTreeImporter` (local backend, wraps `*tree_persist.Intent`) implements both directly from fields it already carries. `JsonTreeImporter`/`XmlTreeImporter` (used only for synced `running`/device data via netconf/gnmi sync, never for real intents) grow trivial stubs returning `false`/`nil`. A future config-server-backed importer implements `GetOrphan()` from `Config.Spec.Lifecycle.DeletionPolicy == DeletionOrphan` and `GetSensitivePaths()` from the joined `SensitiveConfig.Spec.SensitivePaths` (see field mapping and Known limitations).

**`cache.Client`** (`pkg/cache/cache.go`) — every read-side method, `"running"` included, generalizes its return/element type from `*tree_persist.Intent` to `importer.ImportConfigAdapter`; `"running"` moves to two new methods:

```go
type Client interface {
	InstanceCreate(ctx context.Context, cacheInstanceName string) error
	InstanceDelete(ctx context.Context, cacheInstanceName string) error
	InstanceClose(ctx context.Context, cacheInstanceName string) error
	InstanceExists(ctx context.Context, cacheInstanceName string) bool
	InstancesList(ctx context.Context) []string

	InstanceIntentsList(ctx context.Context, cacheInstanceName string) ([]string, error)
	InstanceIntentGet(ctx context.Context, cacheName string, intentName string) (importer.ImportConfigAdapter, error)
	InstanceIntentModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
	InstanceIntentDelete(ctx context.Context, cacheName string, intentName string, IgnoreNonExisting bool) error
	InstanceIntentExists(ctx context.Context, cacheName string, intentName string) (bool, error)
	InstanceIntentGetAll(ctx context.Context, cacheName string, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error)

	// running: always in-process/in-memory, independent of Cache.Type.
	InstanceRunningGet(ctx context.Context, cacheName string) (importer.ImportConfigAdapter, error)
	InstanceRunningModify(ctx context.Context, cacheName string, intent *tree_persist.Intent) error
}
```

**`CacheClientBound`** (`pkg/cache/cacheClientBound.go`) mirrors the same split, unchanged in every other respect — `CacheClientBoundImpl`/`NewCacheClientBound` stay pure pass-throughs, no backend logic of their own:

```go
type CacheClientBound interface {
	InstanceCreate(ctx context.Context) error
	InstanceDelete(ctx context.Context) error
	InstanceExists(ctx context.Context) bool
	IntentsList(ctx context.Context) ([]string, error)
	IntentGet(ctx context.Context, intentName string) (importer.ImportConfigAdapter, error)
	IntentModify(ctx context.Context, intent *tree_persist.Intent) error
	IntentDelete(ctx context.Context, intentName string, IgnoreNonExisting bool) error
	IntentExists(ctx context.Context, intentName string) (bool, error)
	IntentGetAll(ctx context.Context, excludeIntentNames []string, intentChan chan<- importer.ImportConfigAdapter, errChan chan<- error)
	InstanceClose(ctx context.Context) error

	RunningGet(ctx context.Context) (importer.ImportConfigAdapter, error)
	RunningModify(ctx context.Context, intent *tree_persist.Intent) error
}
```

**Amendment (see below the fold):** `InstanceRunningGet`/`RunningGet` originally returned `*tree_persist.Intent` — see "Amendment: `InstanceRunningGet` returns `importer.ImportConfigAdapter`" for why that was corrected before implementation landed.

**New `cache.Client` implementation** (`pkg/cache/configserver.go` or similar, package `cache`) selected by a new `case "config-server":` in `Server.createCacheClient` (`pkg/server/cache.go`), alongside a new `s.config.Cache.Type` value and whatever connection settings the config-server-side local RPC needs (extends `config.CacheConfig`, `pkg/config/datastore.go`). It composes two independent things behind the one `Client` interface:

- A config-server client for the `InstanceIntent*` methods — unary `Get`/`List` calls against the `controller`'s new local surface (see call-site behavior below).
- A minimal in-memory, per-instance map for `InstanceRunning*`, keyed by `cacheName` the same way `LocalCache` scopes its own state — created/cleared alongside `InstanceCreate`/`InstanceDelete`. `LocalCache.InstanceRunningGet`/`InstanceRunningModify` become trivial passthroughs to the same disk-backed mechanism `InstanceIntentGet`/`InstanceIntentModify` already use, so the local backend's behavior for `running` is unchanged — it simply becomes unreachable via the intent-name-keyed path.

### `InstanceIntent*` behavior on the config-server-backed `Client`

One unary `Get(target, name)` / `List(target)` RPC pair (per the contract above) covers all five real-intent methods, unconditionally — no `intentName == consts.RunningIntentName` branch anywhere, because `running` is never asked of this backend through this door:

- `InstanceIntentGet` calls `Get`, wraps the result in an `importer.ImportConfigAdapter`.
- `InstanceIntentGetAll` calls `List`, then ranges over the results sending each into `intentChan`, closing `intentChan`/`errChan` when done or on `ctx.Done()` — the fan-out adapter lives inside this method, mirroring `LocalCache.InstanceIntentGetAll`'s existing shape (`pkg/cache/local.go:63-99`).
- `InstanceIntentsList` also calls `List`, mapped down to names only — `client.List` always returns full objects on the config-server side (confirmed via `Transactor.ListConfigsPerTarget`), so there is no cheaper name-only mode to prefer; both list-shaped methods share one call path, consumed differently.
- `InstanceIntentExists` calls `Get` and maps "not found" to `(false, nil)`, matching the existing `Client` contract's meaning of that return; any other error propagates as `(false, err)`. (No production call site exercises this method today — it is mapped for interface completeness.)
- `InstanceIntentModify`/`InstanceIntentDelete` are unconditional silent no-ops, always returning `nil` — config-server/kube-api owns writes for real intents, and generic code (`lowlevelTransactionSet` in `pkg/datastore/transaction_rpc.go`) has nothing actionable to do when these are called under this backend. No warning log: this is expected, steady-state behavior, not an error condition.
- `excludeIntentNames` is accepted for interface compatibility but is always a no-op in practice: config-server has no `"running"` `Config` resource, so it can never appear in a `List` result to begin with.

### Field mapping (`Config` → `importer.ImportConfigAdapter`)

| Adapter method | config-server source | Notes |
|---|---|---|
| `GetName()` | `metadata.name`/`namespace` via `config.GetGVKNSN` | direct |
| `GetPriority()` | `Config.Spec.Priority` (`int32`) | direct |
| `GetNonRevertive()` | `!cfg.IsRevertive()` from `Config.Spec.Revertive *bool` | inverted; defaults to revertive (`true`) if unset |
| `GetOrphan()` | `Config.Spec.Lifecycle.DeletionPolicy == DeletionOrphan` (`cfg.Orphan()`) | direct |
| `GetSensitivePaths()` | `SensitiveConfig.Spec.SensitivePaths []string`, joined to `Config` **by name**, parsed with `sdcpb.ParsePath` (keyless only) | real field, on config-server's paired `sensitive` branch only — see Known limitations |
| `GetDeletes()` | none | always empty `*sdcpb.PathSet` — see Known limitations |
| Value payload | `Config.Spec.Config []ConfigBlob{Path string, Value runtime.RawExtension}` | raw path + raw JSON; parses cleanly into the same shape `JsonTreeImporter` already consumes |

### `"running"` call sites

`replaceIntent` (`pkg/datastore/transaction_rpc.go:101`) calls `RunningGet` instead of `IntentGet(ctx, consts.RunningIntentName)`, and passes the result straight to `root.ImportConfig(...)` — same as the real-intent path, no local wrapping. `writeBackSyncTree` (`pkg/datastore/transaction_rpc.go:476`) calls `RunningModify` instead of `IntentModify`. Neither passes `consts.RunningIntentName` through the generic real-intent surface anymore. `GetIntent`'s own `"running"` branch (`pkg/datastore/intent_rpc.go:66`) is unaffected — it already bypasses the cache entirely and reads the in-memory `syncTree` directly.

## Amendment: `InstanceRunningGet` returns `importer.ImportConfigAdapter`

The original version of this ADR gave `InstanceRunningGet`/`RunningGet` the concrete return type `*tree_persist.Intent`, on the reasoning that `"running"` is structurally different from a real `Intent` (in-process only, single producer/consumer, no second backend representation), so wrapping it in `importer.ImportConfigAdapter` looked like unjustified abstraction over one implementation.

That reasoning is correct about the *domain* (`"running"` is still not an `Intent` — different accessor names, different storage, excluded from `InstanceIntentGetAll`, unchanged by this amendment) but wrong about what the *return type* is for. `importer.ImportConfigAdapter` isn't "the intent-shaped adapter" — it's the mechanical contract every `Client` read method uses to hand back "data ready for `Tree.ImportConfig`," independent of how many concrete producers exist behind it. Under the original signature, every caller of `RunningGet` had to separately know `"running"` is proto-backed and wrap it themselves (`treeproto.NewProtoTreeImporter(runningIntent)` in `replaceIntent`) — the exact backend-representation leak `InstanceIntentGet` was designed to avoid, just relocated to the call site instead of removed.

The fix: `InstanceRunningGet`/`RunningGet` return `importer.ImportConfigAdapter`, and each `Client` implementation wraps its own `*tree_persist.Intent` before returning it — `LocalCache` and `ConfigServerCache` both already have the value in hand, so the wrap is trivial in both. `transaction_rpc.go` no longer imports `treeproto` for this purpose at all.

## Known limitations

**`SensitivePaths` is real and mapped, not a gap** (revised — earlier drafts of this ADR, researched against config-server's `main` branch, wrongly concluded no source exists at all). Config-server's paired `sensitive` branch (config-server#441) adds a `SensitiveConfig` CR, one per `Config`, keyed by name, produced by a `configresolver` reconciler that derives `SensitivePaths` automatically from which leaves resolved from a `secret::name::key` reference — not a manual per-path declaration, so there's no separate source of ambiguity about which paths qualify. `configresolver` is registered in the same reconciler set (`pkg/reconcilers/all`) and runs in the same colocated `controller` process/manager as the reconciler that already serves `Config` locally, so `SensitiveConfig` is reachable through the identical local-cache read path this ADR defines — a same-process join by name, not a new RPC or a new trust boundary. Because the value is re-derived from durable CR state on every List/Get (including cold-start rebuild), there is no restart-survival gap to design around: recomputing from `SensitiveConfig` on every read *is* the persistence. The schema-level `sdcio-ext:sensitive` baseline (`docs/adr/0003-yang-extension-schema-sensitive-baseline.md`) is backend-agnostic and unaffected either way. This capability depends on the `sensitive` branch pair landing — see the Scope note above — but is not hypothetical or speculative; it's the paired, in-flight counterpart to this very data-server checkout.

**`ExplicitDeletes`/`GetDeletes()` is always empty** for config-server-owned intents — confirmed, but not for the reason earlier drafts of this ADR gave (that config-server has no way to manually revert non-revertive config; it does). Config-server has a live, already-on-`main` mechanism for exactly that: the `TargetClearDeviation` subresource (`cleardeviation`, driven by `kubectl-sdc revert`), which builds a `TransactionIntent` with **`RevertPaths`** set — a third, distinct field from `.Deletes`. `RevertPaths` doesn't tombstone anything directly; it carves specific paths out of an otherwise non-revertive intent's "don't auto-correct drift" behavior for one transaction, so the tree's normal revertive diff/update logic re-applies there (`pkg/tree/api/non_revertive_info.go`). It's read straight off the live `TransactionSetRequest` into `TreeContext.NonRevertiveInfo()` — never through `ImportConfigAdapter` — and today's only implementation, `ProtoTreeImporter`, has no `GetRevertPaths()` either, so this was already true before this design and needs no adapter method or field-mapping row under any backend: a one-shot "treat these paths as revertive for this call" hint has nothing to replay after the fact. Separately, the transient "revert running" path (`pkg/server/transaction.go:62-66`, synthetic `revrun` intent) also never persists, for the same non-replay reason. Checked across `main`, the paired `sensitive` branch, and the current `configmanager.go`/`transactor.go` rewrite: none of them ever populate `.Deletes` on a real intent's `TransactionIntent`. (An older, structurally different branch, `target-rework`, does populate `.Deletes` on a synthetic `deviation:<name>` intent — it doesn't match the shape of either `main` or `sensitive`'s current transactor and isn't part of the branch pair this ADR targets, so it's noted here for completeness rather than treated as a live mechanism.)

Only `ExplicitDeletes` remains a limitation, and it doesn't block landing this backend: config-server's real revert-capable write path uses `RevertPaths`, which needs no durability story at all. If a genuine need for `ExplicitDeletes` durability materializes later (e.g. a planned, not-yet-implemented "expose `ExplicitDeletes` to the user as part of the intent" feature enabling a lowest-precedence "delete everything unmanaged under `/`" cleanup intent), the right vehicle is a real CRD field feeding a proper protocol field — the same path `SensitivePaths` itself took into `sdc-protos` — not a side-channel workaround such as a data-server-owned annotation on the `Config` resource.

## Migration / compatibility

`Cache.Type: local` requires zero changes to existing deployments — every interface change above is either a pure return-type widening that `LocalCache` already satisfies (return `importer.ImportConfigAdapter` instead of `*tree_persist.Intent`; `ProtoTreeImporter` already carries every field the two new accessor methods need) or a new method pair (`InstanceRunningGet`/`InstanceRunningModify`) that `LocalCache` implements as thin passthroughs to its existing disk-backed store. No config, data, or on-disk format migration is needed for the local backend.

## Call sites an implementation PR will touch

- `pkg/cache/cache.go` — `Client` interface (return-type change + new `InstanceRunning*` methods)
- `pkg/cache/cacheClientBound.go` — `CacheClientBound` interface + `CacheClientBoundImpl` forwarding methods
- `pkg/cache/local.go` — `LocalCache` adapts to the new return type; adds `InstanceRunningGet`/`InstanceRunningModify` passthroughs
- `pkg/cache/configserver.go` (new) — the config-server-backed `Client` implementation
- `pkg/server/cache.go` — new `case "config-server":` in `createCacheClient`'s switch (the only acceptable branch point, per the repo's no-tight-coupling rule)
- `pkg/config/datastore.go` — `CacheConfig` grows whatever connection settings the config-server-backed client needs
- `pkg/tree/importer/import_config_adapter.go` — `ImportConfigAdapter` interface (+2 methods)
- `pkg/tree/importer/proto/proto_tree_importer.go` — implements the 2 new accessors from existing `tree_persist.Intent` fields
- `pkg/tree/importer/json/json_tree_importer.go`, `pkg/tree/importer/xml/xml_tree_importer.go` — stub the 2 new accessors (`false`/`nil`)
- `pkg/datastore/intent_rpc.go` (`GetIntent`) and `pkg/datastore/transaction_rpc.go` (`replaceIntent`, `forEachIntent`, `LoadAllButRunningIntents`, `writeBackSyncTree`) — consume the new `ImportConfigAdapter` return type; `replaceIntent`/`writeBackSyncTree` switch to `RunningGet`/`RunningModify`
- `pkg/datastore/datastore_rpc.go` (`populateSensitivePathIndex`'s `IntentGetAll(nil)` call) — stops incidentally seeing `running`, now that `running` is out of `IntentGetAll` entirely
- `mocks/mockcacheclient/{client,clientbound}.go` — regenerate for both interface changes
- `pkg/server/datastore_test.go` and other existing test doubles that construct mock expectations against the changed signatures — real fallout, but mechanical / implementation-PR-level, not a further design decision
