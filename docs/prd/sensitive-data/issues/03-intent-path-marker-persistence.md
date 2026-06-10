# Intent path-marker persistence (Phase 2)

## Parent

[PRD: Sensitive Data Support](../PRD.md)

## What to build

Extend `TransactionSet` to accept, validate, and atomically persist a `sensitive_paths` slice alongside the intent blob in `sdcio/cache`. This is the write side of Phase 2 — the read side (union at render time) is a separate issue.

**API change**

`TransactionSetRequest.Intent` gains `repeated string sensitive_paths`. Callers submit keyless XPath strings (e.g. `/bgp/neighbors/auth-password`). Paths with key predicates (e.g. `/bgp/neighbors[peer=10.0.0.1]/auth-password`) are rejected with a descriptive `InvalidArgument` gRPC error — callers must strip keys before submitting.

**Validation in `pkg/datastore/transaction_rpc.go`**

Before any write:
- Parse each path in `sensitive_paths`
- Reject (error) if any path element contains key predicates
- Reject if a path is empty or malformed

**Atomic persistence**

`cacheClient.IntentModify` (or equivalent) must write the `sensitive_paths` slice to the cache intent record's `sensitive_paths` field atomically with the intent blob. If the cache write fails, the intent blob write is also rolled back — there must be no window where the blob exists without its slice, or vice versa.

**Intent deletion**

When an intent is deleted (`IntentDelete`), the `sensitive_paths` stored with it are removed along with the blob. No tombstone or residual marker is left.

**Replace intent semantics**

A `replace` intent wipes all prior intents for the datastore. The replacing intent must explicitly supply the `sensitive_paths` it considers sensitive. Sensitive-path slices from deleted intents are removed along with those intents. The responsibility for re-declaring sensitive paths lies with the entity performing the replace. This behaviour must be documented in code and in operator-facing docs.

**Missing slice = non-sensitive**

Existing intents in cache have no `sensitive_paths` slice (field absent). Absence is treated as an empty slice — non-sensitive. No migration is required. This must be explicitly documented to avoid future misinterpretation.

## Acceptance criteria

- [ ] `TransactionSetRequest.Intent.sensitive_paths` is wired from proto through `types.TransactionIntent` to the cache write
- [ ] Paths containing key predicates return `InvalidArgument` with a message identifying the offending path
- [ ] Malformed or empty paths return `InvalidArgument`
- [ ] Valid `sensitive_paths` are stored in the cache intent record and round-trip correctly through `IntentGet`
- [ ] If the cache write fails mid-transaction, the intent blob is rolled back (no partial state)
- [ ] Intent deletion removes the stored `sensitive_paths` slice
- [ ] A replace intent clears prior intents' slices; only the replacing intent's slice remains
- [ ] Existing intents without a slice are readable and treated as non-sensitive (no migration, no crash)
- [ ] Unit test: submit intent with valid sensitive paths, read back via cache, assert round-trip
- [ ] Unit test: submit intent with key-predicate path, assert `InvalidArgument`

## Blocked by

Issue 01 (external deps) — needs `sdcio/cache` first-class `sensitive_paths` field and `sdc-protos` `TransactionSetRequest.Intent.sensitive_paths`.
