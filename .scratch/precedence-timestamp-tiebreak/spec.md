# Spec: precedence timestamp tiebreak

**Status:** ready-for-agent

**Design record:** [`pkg/tree/docs/adr/0001-precedence-timestamp-tiebreak.md`](../../pkg/tree/docs/adr/0001-precedence-timestamp-tiebreak.md). Domain terms in [`pkg/tree/CONTEXT.md`](../../pkg/tree/CONTEXT.md). This spec is the execution plan; the ADR is the "why."

## Problem Statement

When multiple intents (or other owners) set the same leaf at the same priority, the configuration tree currently picks a winner by **insertion order** — whichever owner was appended first to the leaf's variant list. That order depends on how intents are loaded from cache, which is not stable across tree rebuilds, config-server List ordering, or process restarts.

A timestamp field exists on tree updates but is never used in precedence comparison. Intent imports always stamp timestamp `0`. The original product rule — **earlier-created intent keeps precedence when priorities tie** — was never implemented or documented until the recent design session.

Operators can misconfigure two device-specific intents onto one target at the same priority. Even when values happen to match on some leaves, other leaves may disagree, and an unstable winner produces non-deterministic intended configuration between runs.

## Solution

Introduce a single **`ComparePrecedence`** comparison used everywhere a winner is selected on equal priority. Comparison order: lower priority number wins (unchanged); on equal priority, **lower timestamp wins** (earlier-created); on equal timestamp, **ascending owner-name** lexicographic sort.

Timestamps are **per-intent**, in **Unix seconds**, stamped on every leaf at import from `ImportConfigAdapter.GetTimestamp()`. config-server supplies K8s `metadata.creationTimestamp` via a new `created_at` field on the config-read proto. Local cache stores `created_at` on the persisted intent proto, set once on first write.

Same priority with the **same value** remains **benign** — no deviation is emitted (community decision). Same priority with **different values** — the loser is reported as overruled. YANG choice-case resolution applies the same tiebreak using the winning leaf's metadata within each branch.

Implementation proceeds **proto → config-server → data-server**, with data-server able to land first using the zero-timestamp fallback until config-server populates the field.

## User Stories

1. As a network operator, I want precedence among equal-priority intents to be stable across datastore restarts, so that the intended configuration does not flap between competing intents.
2. As a network operator, I want the intent that was created first to win when two intents share the same priority on a leaf, so that misconfigured overlapping intents behave predictably.
3. As a data-server maintainer, I want a single `ComparePrecedence` helper, so that leaf selection, deviation detection, and choice-case resolution cannot drift apart.
4. As a data-server maintainer, I want `ImportConfigAdapter` to expose intent creation time, so that every import path stamps leaves consistently.
5. As a config-server maintainer, I want `ConfigEntry` to carry the Config CR's `creationTimestamp`, so that data-server does not infer creation time from load order.
6. As a data-server maintainer, I want local-cache intents to record `created_at` on first persist, so that tiebreak behaviour is consistent regardless of cache backend.
7. As a data-server maintainer, I want transaction updates on an existing intent to preserve that intent's creation time, so that editing config does not change precedence among equals.
8. As a data-server maintainer, I want brand-new intent names in a transaction to receive `time.Now()` as their provisional timestamp, so that tiebreak is defined even before config-server assigns `creationTimestamp`.
9. As a deviation consumer, I want no deviation when equal-priority owners agree on value, so that benign overlap does not generate noise.
10. As a deviation consumer, I want the lower-timestamp owner to win and the other marked overruled when equal-priority owners disagree, so that deviation reporting matches apply behaviour.
11. As a data-server maintainer, I want running and default owners (timestamp `0`) to lose ties against real intent timestamps, so that operational state does not accidentally override configured intents on equal priority.
12. As a data-server maintainer, I want two owners with timestamp `0` to tiebreak by ascending owner name, so that defaults and running remain deterministic among themselves.
13. As a data-server maintainer, I want two intents with the same non-zero timestamp to tiebreak by ascending owner name, so that automation creating multiple configs in the same second remains deterministic.
14. As a data-server maintainer, I want YANG choice-case selection to use the same tiebreak when case branch priorities tie, so that choice resolution is not left to map iteration order.
15. As a test author, I want unit tests on `ComparePrecedence` covering priority, timestamp, owner-name, and equal-value deviation skip rules, so that regressions are caught at the highest seam.
16. As a future reader, I want an ADR and glossary entries recording the tiebreak rules, so that the behaviour is not oral history.
17. As a proto maintainer, I want `created_at` added to both config-read and tree-persist intent messages, so that all backends share one field shape.
18. As a data-server maintainer, I want gNMI/netconf running timestamps normalized to Unix seconds on ingest, so that the tree uses one unit internally even though tiebreak rarely involves running at intent priority levels.

## Implementation Decisions

### Precedence comparison

- Add `ComparePrecedence(a, b) int` (or equivalent three-way compare) in the tree types/API layer. Inputs: priority, timestamp, owner name. Return value follows standard sort semantics (negative = a wins, positive = b wins).
- Rules in order: lower priority number wins; equal priority → lower timestamp wins; equal timestamp → ascending owner name wins.
- Replace bare priority `>` / `<` comparisons in `GetHighestPrecedence`, `GetDeviations` overruled selection, and choice-case resolver winner selection with `ComparePrecedence`.
- `GetDeviations`: when comparing overruled entries, skip emitting deviation when expected value equals the winner's value (unchanged community rule); apply `ComparePrecedence` to determine winner before comparing values.

### Timestamp supply

- **`ImportConfigAdapter`**: new `GetTimestamp() int64` accessor. All adapter implementations must provide it.
- **Intent import processor**: stamp `adapter.GetTimestamp()` on every `NewUpdate` call instead of literal `0`.
- **config-server backend**: extend local-read `Document` and gRPC mapping with `CreatedAt int64` (Unix seconds). config-server controller maps K8s `metadata.creationTimestamp` from the Config CR into config-read `ConfigEntry.created_at`. config-server importer passes it through `GetTimestamp()`.
- **Local cache backend**: add `created_at` to `tree_persist.Intent`. Set on first `IntentModify` if unset; preserve on subsequent modifies. Proto tree importer returns it via `GetTimestamp()`.
- **JSON/XML importers** (running/device sync only): return `0` — running uses high priority numbers and does not participate in intent-intent ties.
- **Transaction path**: when building updates for an intent that already exists in cache, use that intent's stored `created_at`; for a new intent name, use current time in Unix seconds.

### Choice resolver

- Extend choice-case internal structures beyond bare `int32` branch priority to carry the winning leaf's timestamp and owner (or a precomputed precedence tuple) from branch walks.
- Branch priority aggregation (`GetHighestPrecedenceValueOfBranch` or successor) must return enough information for case-level `ComparePrecedence`, not only the minimum priority integer.
- When two cases tie on branch priority, compare using the winning leaf's timestamp and owner within each case.

### Cross-repo contract

- **sdc-protos** (first): add `int64 created_at = N` to `config_read.ConfigEntry` and `tree_persist.Intent`. Document as Unix seconds, intent creation time.
- **config-server** (second, blocked by proto): populate from K8s `metadata.creationTimestamp`.
- **data-server** (third, blocked by proto for full integration): consume field; graceful degradation when `created_at == 0` uses owner-name fallback per ADR.

### Units

- Standardize on **Unix seconds** for intent tiebreak timestamps.
- Normalize gNMI notification timestamps and netconf/noop nanosecond stamps to seconds when stored on running updates (tiebreak rarely applies at running priority, but one unit avoids confusion).

### Documentation

- ADR and `pkg/tree/CONTEXT.md` already written; implementation must conform to them.

## Testing Decisions

- **Primary seam**: unit tests on `ComparePrecedence` — table-driven cases for priority win, timestamp win (lower wins), owner-name win, and full chain. No tests on internal slice iteration order.
- **Secondary seams**: thin integration tests that two same-priority intents with different timestamps produce stable `GetHighestPrecedence` winner regardless of import order (import B before A, then A before B — same winner).
- **Deviation seam**: equal priority + same value → no overruled deviation; equal priority + different values → overruled on loser.
- **Choice resolver seam**: two cases with equal branch priority — winner follows timestamp of branch's winning leaf, not map order.
- **Adapter seam**: config-server and proto importers return expected `GetTimestamp()`; local cache preserves `created_at` across modify.
- **Regression**: existing tree precedence tests continue to pass; update any that implicitly relied on insertion order for equal-priority scenarios.
- Prior art: `pkg/tree/entry_test.go`, `pkg/tree/api/leaf_variants` behaviour tests, `pkg/cache/configserver_test.go` field-mapping tests.
- Final check: `go test ./pkg/tree/...` and affected datastore/cache packages pass.

## Out of Scope

- GitHub issue #479 (periodic NETCONF commits every 30s) — separate investigation.
- Warning or error when same-priority intents overlap with matching values — benign per community call.
- Per-leaf timestamps for intents (only per-intent creation time).
- Using `metadata.generation` or `resourceVersion` as tiebreak source.
- Last-applied / first-successful-push timestamp (heavier; rejected in design).
- Changing numeric priority semantics (lower still wins).
- config-server write-path or reconciler changes beyond exposing `creationTimestamp` on read.

## Further Notes

- Cross-repo tickets (proto, config-server) may be tracked in those repositories; data-server tickets should declare blocking edges explicitly (e.g. "Blocked by: proto ticket" as a note until sibling work lands).
- data-server can implement `ComparePrecedence` and adapter wiring with `created_at = 0` fallback before config-server ships — behaviour matches today for zero timestamps but becomes deterministic via owner-name sort instead of insertion order.
- This spec was synthesized from a `/grill-with-docs` session; consult the ADR for rejected alternatives rather than re-deriving "why" from this spec's "what."
- Recommended ticket split for `/to-tickets`: (1) sdc-protos `created_at`, (2) config-server mapping, (3) data-server `ComparePrecedence` + leaf/deviation wiring, (4) data-server choice resolver + adapter timestamp plumbing, (5) data-server transaction/local-cache timestamp persistence.
