# Data server

Holds the intended configuration of network devices (as intents) and keeps a view of what is actually configured on each device.

> Interim glossary. Once the `synced-gate` work (PR #501) lands, merge these terms into `pkg/datastore/CONTEXT.md` so the datastore has a single glossary. That file also uses "revert" for the pre-replace snapshot of a transaction, which is a different concept from **Drift revert** below.

## Language

**Running**:
The configuration actually present on the device, as last observed by a Sync. Held in the sync tree under the reserved `running` owner and priority.

**Sync**:
Reading the device's configuration through its target (NETCONF get-config, gNMI Get, or a gNMI subscription) and merging it into Running. Every Sync has a configured scope (its sync paths); a stream Sync commits incremental ticks.

**Drift**:
Running differs from the highest-precedence intended value of a revertive intent, e.g. someone logged in to the device and changed a value by hand, or a second orchestrator overwrote it. A change to Running that equals the intended value is not Drift, and non-revertive intents never produce Drift.

**Unmanaged config**:
A value present in Running that no intent defines and that no delete-path intent covers, e.g. set by hand or already on a brownfield device. There is no expectation for it, so it is never Drift and is never corrected.

**Delete-path intent**:
An intent that declares a path (e.g. `/`) to be deleted, with a priority just above Running. It makes Unmanaged config under that path expected to be absent, so it becomes Drift and is corrected. Every intent that defines a value has higher precedence, so those values win over the delete.

**Drift revert**:
Pushing the intended state back to the device to correct Drift. Only triggered by a Sync; it is the only mechanism that corrects Drift.

**Deviation**:
A reported (not corrected) difference between intents and Running, streamed to `WatchDeviations` clients. Reporting only; never causes a Drift revert.

**Outstanding drift revert**:
A per-datastore, in-memory marker that a Drift revert is still owed: set when a Drift revert fails (preparing it or applying it to the device), cleared only when the Drift revert succeeds. Not persisted; a restart starts with a full first Sync, which re-evaluates everything.

**Touched entries**:
The entries whose flags a Sync set or changed (marked deleted, newly created, updated, or restored from a delete mark). Post-sync clean-up work is limited to these.

**Revert scope**:
The part of the tree a Drift revert loads and compares for one Touched entry: the entry's parent (the entry itself if it is a top level entry, the list entry for a key level entry that does not carry all keys yet). If the scope is an element of a case, it is widened to the entry that owns the choice, since the other cases compete with it. Scopes are collected before the Sync removes entries marked deleted, so removed entries are covered too. Scopes are not pruned against each other. The root as a scope means the whole tree is checked.
