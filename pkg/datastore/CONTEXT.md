# pkg/datastore — Context

## Glossary

### Synced

**Synced** is a one-shot, per-`Datastore` latch: it becomes `true` the first
time Running has completed a successful sync cycle from the target for
*every* configured sync (`d.config.Sync.Config`), and never reverts
afterwards.

- A datastore with zero configured syncs (nil `Sync`, or an empty
  `Sync.Config`) is trivially Synced from the start.
- `noop`-backed datastores are also trivially/immediately Synced: `noop`
  never talks to a device, so `noop.AddSyncs` calls
  `RunningStore.MarkSynced` for every sync entry it discards instead of
  starting anything.
- For real targets, each sync mechanism (`StreamSync`, `GetSync`,
  `NetconfSyncImpl`) calls `RunningStore.MarkSynced(name)` once
  **`ApplyToRunning` succeeded** for that mechanism’s cycle — not merely
  on receiving a signal/response from the device (for example, gNMI
  `SyncResponse` alone is not enough until apply succeeds).
- **Empty device snapshots** under configured sync paths count as successful
  cycles when apply succeeds: the device and Running agree there is nothing
  under that scope, and the Synced latch can close on factory-default
  deployments.
  - **Get** and **Netconf** (and **Once**, once implemented): *scoped refresh*
    — `ApplyToRunning` with configured sync paths and a **nil importer** when
    there is no Running content to import.
  - **Stream** initial snapshot: *empty snapshot commit* — `ApplyToRunning`
    with **nil paths** and a **nil importer** when the post-`SyncResponse`
    export is empty (not path-scoped refresh). Ongoing stream ticker /
    incremental commits are unchanged and do not call `MarkSynced`.
- `OnceSync` remains **excluded** until Phase B: it does not apply to Running
  or participate in Synced today.
- The latch survives target reconnects: `AddSyncs` (and therefore sync
  object construction) only happens once, at target construction time, so
  there is nothing to re-arm on reconnect.

**Why it matters:** `TransactionSet` gates on Synced (see `ErrNotSynced`)
immediately before its replace phase and before its merge phase
(`lowlevelTransactionSet`), for both real and dry-run calls. A no-op
transaction (no intents, no replace) is never gated. `TransactionRollback`
is not gated — it only ever runs against a transaction that already passed
the gate. This exists so that a pre-replace Running snapshot (see
`docs/adr/0001-replace-revert-via-oldrunning-gated-on-synced.md`) is only
relied upon once Running is known to reflect the device at least once;
staleness *after* the first sync is accepted, out-of-scope architecture.

See `pkg/datastore/synced.go` for the implementation
(`Datastore.MarkSynced` / `Datastore.Synced`).
