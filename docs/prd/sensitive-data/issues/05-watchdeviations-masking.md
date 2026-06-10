# WatchDeviations sensitive-path masking

## Parent

[PRD: Sensitive Data Support](../PRD.md)

## What to build

Mask sensitive leaf values in the `WatchDeviations` stream. Both `ExpectedValue` and `CurrentValue` in each streamed `WatchDeviationResponse` must be replaced with `***` when the leaf's path is in the sensitive set — whether that sensitivity comes from the schema flag (`IsSensitive`) or from any intent's stored `sensitive_paths` slice.

**Where the change lives**

The deviation manager loop (`pkg/datastore/deviations.go`, `DeviationMgr`) deep-copies the sync tree, loads all intents, calls `ops.GetDeviations`, then streams results via `SendDeviations`. The sensitive-path union must be built once per deviation cycle (not per streamed message) and passed into both the deviation detection walk and the emit step.

**Union building**

Reuse the same `ops.LoadSensitivePathSet` helper introduced in issue 04. Load it from the same intent set that `DeviationMgr` already loads. Schema-level sensitivity (`IsSensitive`) is checked inline at the leaf level, same as in the render ops.

**Masking at emit**

In `SendDeviations`, before writing `ExpectedValue` and `CurrentValue` into the response message, apply:

```
if !exposeSensitive && (ops.IsSensitive(leaf) || sensitivePathSet[ops.KeyPrunedPath(leaf)]) {
    expectedValue = "***"
    currentValue  = "***"
}
```

`WatchDeviationsRequest` does not currently have an `include_sensitive` flag. Whether to add one is a decision for this issue — the PRD defers it but the bypass hook is already designed to be a single point in the pipeline.

**No `include_sensitive` on `WatchDeviationsRequest` in v1**

Streaming bypass is a distinct auth problem (the stream runs continuously; the caller's privilege level needs to be checked per-message or at connection time). For v1, always redact and leave the bypass as a follow-up when mTLS cert roles are implemented.

## Acceptance criteria

- [ ] `WatchDeviationsResponse`: `ExpectedValue` and `CurrentValue` are `***` for any leaf whose path is sensitive (schema flag or any intent's stored slice)
- [ ] Non-sensitive leaves are unaffected
- [ ] The union is built once per deviation cycle, not per leaf or per message
- [ ] Schema-flag sensitivity (`IsSensitive`) and path-slice sensitivity (`SensitivePathSet`) are both checked
- [ ] No `include_sensitive` bypass added to `WatchDeviationsRequest` in this issue (documented as follow-up)
- [ ] Integration test: intent marks `/foo/bar` sensitive; running value differs from expected; verify both deviation values are `***` in the stream

## Blocked by

Issue 04 (path-marker union at render time) — `ops.LoadSensitivePathSet` and `ops.KeyPrunedPath` must exist; `IsSensitive` already exists from issue 02.
