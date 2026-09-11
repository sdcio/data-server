# Spec: Fix flaky "datastore is locked" failures in partial deviation revert

**Status:** done

**Effort:** cross-repository — data-server (this ticket, tracking only),
config-server (fix), integration-tests (fix)

---

## Problem Statement

CI run [34482168553](https://github.com/sdcio/data-server/actions/runs/34482168553/job/102888410883)
(PR#471 "Config server cache backend", commit `cf88756`) failed the
`integration-tests (config-server cache backend) / setup-clab-cluster-and-test`
job with exit code 5 on the `03-deviations` Robot suite. Five tests failed,
all in the "Partially Revert Deviations by Filter Path and Verify remaining
deviations" group (`intent1`..`intent5`, SRL suite):

```
kubectl sdc deviation --deviation config-intent5-srl --filter-path /interface[name=ethernet-1/5] --revert
→ rc = 1
→ Error: rpc error: code = Aborted desc = datastore is locked, other action is ongoing:
         intent5-srl: rpc error: code = Aborted desc = datastore is locked, other action is ongoing
```

## Root cause

Traced in `data-server.log` (artifact `03-deviations-logs/data-server.log`,
datastore `default.srl3`, 13:59:55Z): transaction `68692b55-...` (a normal
config-apply, most likely from config-server's `TargetConfigController`
reconciler) held `Datastore.dmutex` (`pkg/datastore/transaction_rpc.go`) for
its `TransactionSet` call. In the same second, transaction `bc1de355-...` —
the `TransactionSet` underlying the failing `kubectl sdc deviation --revert
--filter-path` call — tried to acquire the same lock via `TryLock()`, lost the
race, and was rejected with `ErrDatastoreLocked` (`codes.Aborted` on the wire).
`68692b55` completed normally moments later. **This is not a deadlock or a
hang** — `dmutex`'s fail-fast `TryLock` semantics predate this branch (commit
`536f770`, "Add Transactionality") and are working as designed; it is
legitimate, expected lock contention between two independent callers hitting
the same datastore back-to-back.

Corroborating throughput evidence: across the 20-minute suite, each of
`srl1`/`srl2`/`srl3` processed roughly one `TransactionSet` every ~300–400ms on
average (10,716 `"config transaction confirmed"` log lines across ~3
datastores over ~20 min, `data-server-controller.log`) — the datastore is
already densely packed with back-to-back transactions, so any operation that
doesn't retry on a rejected `TryLock` is inherently flaky under this suite's
concurrency, independent of `Cache.Type`.

**Why this surfaced now, on the config-server-cache-backend job specifically:**
this exact bug class was already hit and partially fixed on the *local-cache*
job earlier in this same PR's history — see
`integration-tests` commit `c69c07e` ("fix(deviations): retry Delete Deviation
on transient datastore-locked errors", closing
`integration-tests/.scratch/last-applied-snapshot-write-at-apply/issues/08`).
That ticket named **two** originally-failing test groups — "Reject Deviations"
and "Partially Revert Deviations by Filter Path" — but the landed fix only
wrapped the keyword behind the first group (`Delete Deviation` in
`tests/Keywords/deviation.robot`). The second group's keywords,
`Partial Revert Deviations For Intent by Interface`
(`tests/03-deviations/22-srl-nonrevertive.robot`) and
`Partial Revert Deviations For Intent by Admin State`
(`tests/03-deviations/21-sros-nonrevertive.robot`), were missed and remained
unwrapped — exactly the two keywords behind this run's failures. It was
latent/masked on earlier config-server-cache-backend runs by unrelated,
already-fixed regressions (see `.scratch/config-server-cache-backend-ci-fix/`);
once those cleared, this pre-existing gap became reachable and visible here.

**Ruled out:** we initially suspected `Cache.Type: config-server` itself was
the regression — `TransactionSet`'s apply loop now makes a `List` RPC
(`LoadAllButRunningIntents`) plus one `Modify`/`Delete` RPC *per intent* to
config-server (`pkg/cache/docs/adr/0001`, `0003`), all while holding
`dmutex`, replacing what used to be instant local disk/memory ops under
`Cache.Type: local`. This is a real, documented latency cost (ADR 0003 itself
flags per-intent RPC batching as "a legitimate future optimization"), but we
could not find or produce a measurement proving it materially worsened this
specific flake: data-server's JSON logs carry only second-resolution
timestamps and neither `data-server-controller.log` nor `api-server.log`
(config-server's containers) log any per-RPC duration for the local
`ConfigSnapshotService` surface. Absent that evidence, and given the fix
below resolves the failure at the protocol-contract layer regardless of
`Cache.Type`, we did not pursue a data-server-side change. If per-transaction
latency under `Cache.Type: config-server` ever needs to be *measured* rather
than inferred, adding duration instrumentation around
`ConfigServerCache.InstanceIntent{Modify,Delete,GetAll}` would be the
prerequisite — noted here as a possible future ticket, not undertaken now.

## Solution

`codes.Aborted` is gRPC's documented status for exactly this situation: ["the
client should retry at a higher level (e.g., when a client-specified
test-and-set fails, indicating the client should restart a read-modify-write
sequence)"](https://grpc.io/docs/guides/status-codes/). data-server's
`TryLock` *is* that test-and-set; nothing was applied server-side when it
loses the race, so replaying the same request is always safe. The retry
obligation belongs to the caller, and config-server already has an established,
tested convention for this exact classification:
`pkg/sdc/target/manager/transactor.go`'s `isRecoverableGRPCError` (now
`dsclient.IsRecoverableError`) classifies `codes.Aborted`/`codes.ResourceExhausted`
as recoverable, and `TargetConfigController`'s reconcile loop
(`pkg/reconcilers/targetconfig/reconciler.go`) already requeues with a 500ms
backoff on a recoverable error for its own `TransactionSet` calls — which is
exactly why the reconciler's *own* transactions self-heal through this same
contention (10,716 succeeded vs 125 failed in this run's logs) while the
deviation-revert path didn't.

Three changes, in the two sibling repos that actually needed them:

1. **config-server** — `executeClearDeviationTx`
   (`apis/config/target_helpers.go`), the function every `kubectl sdc
   deviation --revert` call reaches via the `cleardeviation` k8s subresource,
   had no retry at all: it called `TransactionSet`/`TransactionConfirm` once
   and returned whatever error it got straight through to the HTTP caller.
   Moved `isRecoverableGRPCError`'s classification to the shared
   `pkg/sdc/dataserver/client` package (`IsRecoverableError`, the lowest layer
   both `apis/config` and `pkg/sdc/target/manager` already depend on, avoiding
   an import cycle) and wrapped both calls in a bounded retry loop
   (`retryOnRecoverable`, 5 attempts, 500ms fixed backoff — matching the
   reconciler's own `RequeueAfter`). This is a synchronous, blocking-caller
   analogue of what the reconciler already does by requeueing.

2. **integration-tests** — completed the fix `c69c07e` started: wrapped
   `Partial Revert Deviations For Intent by Interface`
   (`22-srl-nonrevertive.robot`) and `Partial Revert Deviations For Intent by
   Admin State` (`21-sros-nonrevertive.robot`) in
   `Wait Until Keyword Succeeds ${eventual_timeout} ${retry}`, extracting a
   `Run Partial Revert Deviations by <Interface|Admin State>` helper each —
   same shape as `c69c07e`'s `Run Deviation Revert` extraction. This is a
   CI-level safety net independent of the config-server fix (defense in depth,
   and it's what ticket 08 originally asked for and missed).

3. **data-server** — this ticket. No code change needed in this repo; the
   root cause and fix live entirely in the two sibling repos.

## Why not fix data-server's locking model instead

Considered and rejected: making `Datastore.dmutex` block-with-timeout instead
of `TryLock`-fail-fast, so contention resolves server-side without the client
seeing an error at all. Rejected because:

- The fail-fast semantics predate this branch and look intentional (single
  active transaction per datastore, immediate feedback rather than queuing).
- `codes.Aborted` already has defined, standard retry semantics on the client
  side per gRPC's own guidelines — solving it there doesn't require changing
  server behavior that other callers may already depend on.
- A blocking/queueing model risks head-of-line blocking and cascading
  timeouts under genuine sustained overload, a bigger behavior change for a
  problem the protocol already has a defined, safe answer to.

## Out of scope

- Adding per-RPC duration instrumentation to `ConfigServerCache` /
  `ConfigSnapshotService` (noted above as a possible follow-up if this needs
  to be measured rather than inferred later).
- Batching the per-intent `Modify`/`Delete` calls in `lowlevelTransactionSet`
  into one RPC (ADR 0003's noted future optimization) — not pursued since we
  couldn't confirm it's the actual contributor here.
- Changing `Datastore.dmutex`'s `TryLock`-fail-fast model.

## Further Notes

Evidence trail (CI run 34482168553, job 102888410883,
`03-deviations-logs/data-server.log` and `data-server-controller.log`):

- 5 test-level `FAIL`s, all `Partially Revert Deviations by Filter Path and
  Verify remaining deviations - intentN`, all `1 != 0` on the
  `kubectl sdc deviation --revert --filter-path` step.
- Exact collision: `default.srl3`, 13:59:55Z, transactions `68692b55-...`
  (held the lock, completed normally) vs `bc1de355-...` (rejected,
  `ErrDatastoreLocked`, this is the failing revert call).
- 102 total `"datastore is locked, other action is ongoing"` occurrences and
  125 total `"config transaction failed"` entries in
  `data-server-controller.log` over the 20-minute run, against 10,716
  successful `"config transaction confirmed"` — i.e. contention is common but
  the reconciler's own retry-on-recoverable already absorbs the vast majority
  of it; only call sites without a retry (the two missed Robot keywords) see
  it as a hard failure.
