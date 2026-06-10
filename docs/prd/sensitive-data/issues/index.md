# Sensitive Data Support — Issue Index

## Issues

| # | Title | Status | Blocked by |
|---|---|---|---|
| [01](./01-external-deps.md) | External deps: goyang fix + sdc-protos + cache field | done | — |
| [02](./02-schema-level-redaction.md) | Schema-level render redaction + admin bypass | done | 01 |
| [03](./03-intent-path-marker-persistence.md) | Intent path-marker persistence | done | 01 |
| [04](./04-path-marker-union-at-render.md) | Path-marker union at northbound render time | done | 02, 03 |
| [05](./05-watchdeviations-masking.md) | WatchDeviations sensitive-path masking | done | 04 |

---

## Issue 01 — External deps

**Status:** done

**PRs:**
- [sdcio/goyang#4](https://github.com/sdcio/goyang/pull/4) — `fix-apply-deviate-exts-replace`
- [sdcio/sdc-protos#123](https://github.com/sdcio/sdc-protos/pull/123) — `feat-sensitive-paths-schema-path`

**Files touched:**
- `goyang/pkg/yang/entry.go` — fix `ApplyDeviate`: `DeviationAdd` appends `Exts`, `DeviationReplace` replaces them (was incorrectly appending for both); mirrors existing `Default` branching
- `goyang/pkg/yang/entry_test.go` — new `TestDeviation` case `"deviate replace replaces existing extensions, does not accumulate"` in addition to the two propagation cases
- `sdc-protos/schema.proto` — `LeafListSchema.sensitive = 24`, `LeafSchema.sensitive = 22`
- `sdc-protos/data.proto` — `GetIntentRequest.include_sensitive = 4`, `BlameConfigRequest.include_sensitive = 2`; `TransactionIntent.sensitive_paths: repeated schema.Path = 11`
- `sdc-protos/tree_persist.proto` — `Intent.sensitive_paths: repeated schema.Path = 8` (was `repeated string`)
- `sdc-protos/sdcpb/schema.pb.go`, `sdc-protos/sdcpb/data.pb.go`, `sdc-protos/tree_persist/tree_persist.pb.go` — regenerated
- `sdc-protos/tree_persist/tree_persist_sensitive_test.go` — round-trip + backward-compat tests
- `data-server/go.mod` — local replace directives (update to tagged PRs after merge)

---

## Issue 02 — Schema-level render redaction + admin bypass

**Status:** done

**Design decisions recorded in issue doc:**
- `RenderOpts` / `XMLRenderOpts` / `XPathRenderOpts` struct hierarchy in `pkg/tree/ops/render_opts.go`
- `SensitivePathSet map[string]bool` in `RenderOpts` — populated from per-intent `sensitive_paths` union (Issue 04); nil/empty = schema-flag-only redaction (Phase 1)
- `KeyPrunedPath(e)` = `e.SdcpbPath().ToXPath(true)` — reuses existing `noKeys` parameter
- Callers to update: `IntentResponseAdapter`, all ops test files, datastore intent/blame RPC layers, server handlers

**Files touched:**
- `pkg/tree/ops/sensitive.go` — new: `IsSensitive`, `KeyPrunedPath`
- `pkg/tree/ops/sensitive_test.go` — new: unit tests for above
- `pkg/tree/ops/render_opts.go` — new: `RenderOpts`, `XMLRenderOpts`, `XPathRenderOpts`
- `pkg/tree/ops/json.go` — `ToJson`/`ToJsonIETF` migrated to `RenderOpts`; redaction added at leaf emit
- `pkg/tree/ops/xml.go` — `ToXML` migrated to `XMLRenderOpts`; redaction added at leaf emit
- `pkg/tree/ops/proto.go` — `ToProtoUpdates` migrated to `RenderOpts`; redaction added per leaf-entry
- `pkg/tree/ops/xpath.go` — `ToXPath` migrated to `XPathRenderOpts`; redaction added per leaf-entry
- `pkg/tree/ops/json_test.go` — migrated to `RenderOpts`; `TestToJsonSensitiveRedaction` added
- `pkg/tree/ops/xml_test.go` — migrated to `XMLRenderOpts`
- `pkg/tree/ops/xpath_test.go` — migrated to `XPathRenderOpts`
- `pkg/tree/api/adapter/intentresponse.go` — added `RenderOpts` field; all `To*` methods forward it; `ToXML`/`ToXPath` use typed opt structs
- `pkg/tree/api/adapter/entryoutputadapter.go` — southbound adapter wraps bools into structs; uses `IncludeSensitive: true` everywhere
- `pkg/tree/processors/processor_blame_config.go` — `BlameConfigProcessorParams` gains `IncludeSensitive`, `SensitivePathSet`; `BlameConfigTask.Run` redacts `Value`/`DeviationValue`
- `pkg/tree/processors/processor_blame_config_test.go` — `TestBlameConfigSensitiveRedaction` added
- `pkg/datastore/intent_rpc.go` — `GetIntent` accepts `exposeSensitive bool`; threads into `IntentResponseAdapter.RenderOpts`
- `pkg/datastore/datastore_rpc.go` — `BlameConfig` accepts `exposeSensitive bool`; threads into `BlameConfigProcessorParams`
- `pkg/datastore/sync.go` — southbound `ToProtoUpdates` call updated to `IncludeSensitive: true`
- `pkg/server/intent.go` — passes `req.GetIncludeSensitive()` to `ds.GetIntent`
- `pkg/server/datastore.go` — passes `req.GetIncludeSensitive()` to `ds.BlameConfig`
- `pkg/utils/testhelper/utils.go` — `GetSchemaClientBoundMarkingLeafSensitive` helper added
- `mocks/mocktreeentry/entry.go` — regenerated (stale `AddChild` → `AddOrGetChild`)

---

## Issue 03 — Intent path-marker persistence

**Status:** done

**Files touched:**
- `sdc-protos/data.proto` — `TransactionIntent.sensitive_paths = 11`
- `sdc-protos/sdcpb/data.pb.go` — regenerated
- `pkg/datastore/types/transaction_intent.go` — `sensitivePaths []string` field; `SetSensitivePaths` / `GetSensitivePaths` accessors
- `pkg/datastore/transaction_rpc.go` — `validateSensitivePaths` (rejects key predicates, empty, slash-less paths); wired into `SdcpbTransactionIntentToInternalTI`; `protoIntent.SensitivePaths` set in `lowlevelTransactionSet` before `IntentModify`
- `pkg/datastore/transaction_rpc_test.go` — `TestTransactionSet_SensitivePathsPersisted` (round-trip tracer bullet); `TestSdcpbTransactionIntentToInternalTI_SensitivePaths` (key-predicate, empty, no-slash, valid-pass-through)

---

## Issue 04 — Path-marker union at northbound render time

**Status:** done

**Files touched:**
- `pkg/tree/ops/sensitive.go` — `UnionSensitivePaths(perIntentPaths [][]string) map[string]bool` added
- `pkg/tree/ops/sensitive_test.go` — `TestUnionSensitivePaths` added (nil, empty, single, union, dedup cases)
- `pkg/datastore/transaction_rpc.go` — `LoadAllButRunningIntents` signature changed to `([]string, map[string]bool, error)`; collects `intent.GetSensitivePaths()` per intent and returns `ops.UnionSensitivePaths(...)` as second value
- `pkg/datastore/sync.go` — call site updated (`_, _, err`)
- `pkg/datastore/deviations.go` — call site updated (`addedIntentNames, _, err`)
- `pkg/datastore/datastore_rpc.go` — `BlameConfig` captures the union from `LoadAllButRunningIntents` and passes it as `BlameConfigProcessorParams.SensitivePathSet`
- `pkg/datastore/intent_rpc.go` — `loadSensitivePathSet(ctx)` helper added (streams all intents via `IntentGetAll`, collects sensitive paths, returns `ops.UnionSensitivePaths`); `GetIntent` calls it before rendering for both the running-intent and single-intent branches, populates `RenderOpts.SensitivePathSet`
- `pkg/datastore/sensitive_path_union_test.go` — new: `TestBlameConfig_CrossIntentSensitivePathRedaction`, `TestGetIntent_CrossIntentSensitivePathRedaction`, `TestGetIntent_NoSensitivePaths_NoRedaction`, `TestGetIntent_RunningIntent_CrossIntentRedaction`

---

## Issue 05 — WatchDeviations masking

**Status:** done

**Files touched:**
- `pkg/tree/ops/sensitive.go` — `IsSensitiveSchemaElem(*sdcpb.SchemaElem) bool` added; `IsSensitive` refactored to call it
- `pkg/tree/ops/sensitive_test.go` — `TestIsSensitiveSchemaElem` added
- `pkg/datastore/deviations.go` — `calculateDeviations` signature changed to `(<-chan, map[string]bool, error)`; `DeviationMgr` captures `sensitivePathSet` and passes it to `SendDeviations`; `SendDeviations` gains `sensitivePathSet map[string]bool` parameter and masks `ExpectedValue`/`CurrentValue` with `***` when sensitive; `isSensitiveDeviation` helper added (checks path-marker set + schema flag)
- `pkg/datastore/deviations_test.go` — new: `fakeDeviationStream` test helper; `TestSendDeviations_PathMarkerSensitiveMasking`, `TestSendDeviations_SchemaFlagSensitiveMasking`, `TestSendDeviations_NonSensitiveNotMasked`, `TestWatchDeviations_SensitivePathMasking` (integration acceptance test)
