# Cache

Owns the `Client` abstraction data-server uses to read and write per-target intent
and device-state data, independent of which backend (`local`, `config-server`)
actually stores it.

## Language

**Intent**:
A named, prioritized piece of northbound-authored config, owned by whichever
system is the sole writer for the active `Cache.Type` (config-server, or
data-server itself under `local`). Read through the `Client.InstanceIntent*`
family.

**Running**:
The synced, on-device configuration state — produced and consumed entirely
in-process (`ops.TreeExport` / `ImportConfigAdapter`), never authored
northbound. Not an `Intent`: it is exempt from the `Client.InstanceIntent*`
family (own accessors, own storage, excluded from `InstanceIntentGetAll`)
under every backend, because it was never config-server's data to begin with.
_Avoid_: calling it an intent, "the running intent" as a `Client`-level type.

**Orphan**:
Deletion policy for an Intent: drop it from the intended store (stop
managing it) without deleting the corresponding config on the device,
leaving that config unmanaged. Wire `Orphan`, internal `onlyIntended` /
`DeleteOnlyIntended`, and config-server `DeletionPolicy=orphan` are the
same bit. _Avoid_: a second meaning for leaf-variant "orphan delete"
(same policy); "orphan" for unowned tree nodes or "Intent not found".

**ImportConfigAdapter**:
The mechanical shape ready to hand to `Tree.ImportConfig` (tree walk plus
import-needed metadata: name, priority, deletes, non-revertive). Returned
directly by Running reads (`InstanceRunningGet`). Intent reads return
`IntentAdapter`, which embeds this shape. _Avoid_: putting Orphan or path
markers on this interface; those are Intent-only (see **IntentAdapter**).

**IntentAdapter**:
The `Client` Intent-read shape: an `ImportConfigAdapter` plus Intent-only
metadata (Orphan, path markers / SensitivePaths). Returned by
`InstanceIntentGet` / `InstanceIntentGetAll`. Implemented by proto- and
Document-backed adapters; never by JSON/XML device/running importers.
_Avoid_: `IntentDescriptor` for this type; conflating with
`IntentResponseAdapter` (northbound GetIntent response that also carries a
Tree Entry).

**ConfigSnapshotService**:
The canonical local gRPC seam name for config-server-backed intent reads and
writes (`Get`/`List`/`Modify`/`Delete`) against `TargetSnapshot` data. _Avoid_:
using the older `ConfigReadService` name for current behavior.
