# Cache

Owns the `Client` abstraction data-server uses to read and write per-target intent
and device-state data, independent of which backend (`local`, `config-server`)
actually stores it.

## Language

**Intent**:
A named, prioritized piece of northbound-authored config — the *desired*
value. Authorship (who decides what an Intent's content should be) belongs
to whichever system is the sole writer for the active `Cache.Type`
(config-server, or data-server itself under `local`). Read through the
`Client.InstanceIntent*` family. _Avoid_: using "Intent" when you mean
last-applied — under `Cache.Type: config-server` these are two different
values with two different owners (see **Last-applied** below).

**Last-applied**:
The value of a named Intent that this datastore last successfully pushed
southbound — updated at the same moment `IntentModify`/`IntentDelete` run
inside `TransactionSet`'s apply loop, for every `Cache.Type`, not gated on
`TransactionConfirm`. Under `Cache.Type: local` this has always been true by
construction (disk write happens at that exact call). Under
`Cache.Type: config-server`, data-server writes this back into config-server
so `Client.InstanceIntent*` reads (backed by `TargetSnapshot.Spec.Configs`)
never lag behind what was actually applied — a deleted Intent must stop
being last-applied at delete-apply time, not at some later, best-effort
snapshot refresh, or the next `LoadAllButRunningIntents` can rehydrate config
that was meant to be gone. _Avoid_: "confirmed" or "acknowledged" (collide
with the separate `TransactionConfirm` RPC step); "expected state" (says
nothing about whether it's desired-or-applied); and "Document" as a domain
concept (a branch-local Go DTO name for the ConfigSnapshotService
interchange shape — not a term in this language).

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

**Namespaced name**:
Under `Cache.Type: config-server` only, the `namespace.name` spelling (split on
the first `.`) used for a Target's datastore identity and for an Intent's
owner / Config identity. The local backend has no namespaces and does not use
this spelling. _Avoid_: treating this as a `Client`-level concept; "GVKNSN" as
the everyday term (wire/comment echo of config-server); implying the local
cache understands dotted names.
