# Schema-level sensitive baseline via YANG extension `sdcio-ext:sensitive`

Leaves and leaf-lists that must never appear in plaintext over the northbound API are marked at the **schema level** using a custom YANG extension (`sdcio-ext:sensitive`) applied through `deviate add` in per-device-profile overlay files. Schema-server reads the extension and exposes `sensitive: bool` on `LeafSchema` / `LeafListSchema`. data-server checks the flag at northbound render time and emits `***` instead of the actual value.

## Why this approach over the alternatives

| Alternative | Reason rejected |
|---|---|
| Sidecar YAML per device-profile | Parallel artefact out of sync with YANG overlays already used for other deviations; two sources of truth for the same schema profile |
| Leaf-name heuristic (`password`, `secret`, …) | Global, implicit, brittle — false positives on unrelated leaves; cannot be scoped per device-profile |
| `DatastoreConfig` YAML `sensitive-paths` list | Per-datastore scope instead of per-schema-profile; duplicated across every datastore that shares the same device model |
| Phase-2 intent path-slice only (no static baseline) | Operators must re-declare sensitivity on every `TransactionSet`; one missed call exposes the secret until the next set |

`deviate add` with a YANG extension is the only mechanism that co-locates sensitivity with the rest of the schema profile, is version-controlled alongside the vendor YANG overlays, and propagates automatically through the existing schema-server load pipeline.

## Key design points

**goyang fork fix.** `ApplyDeviate` in `sdcio/goyang` silently dropped extensions on `deviate add` / `deviate replace` nodes. A one-line fix (`deviatedNode.Exts = append(deviatedNode.Exts, devSpec.Exts()...)`) enables the extension to survive into the parsed `yang.Entry`.

**Extension module.** The extension is defined in `sdcio-extensions.yang` (module name `sdcio-extensions`, conventional prefix `sdcio-ext`), embedded in schema-server so operators do not manage it. The extension name is `sensitive` (statement: `sdcio-ext:sensitive;` — no argument). Prefix resolution (not keyword string match) is used when detecting the extension so overlay files that choose a different prefix still work.

**Proto field.** `bool sensitive = 22` added to `LeafSchema`; `bool sensitive = 24` added to `LeafListSchema` in `sdc-protos`. The existing `encrypted: bool` field is unrelated (future KMS concern) and is left unchanged.

**data-server render ops.** `ops.IsSensitive(e api.Entry) bool` reads the schema flag for both leaf and leaf-list. It is called inside the format-specific render functions (`ToJson`, `ToJsonIETF`, `ToProtoUpdates`, `ToXML`, `ToXPath`) which all receive a `RenderOpts` / `XMLRenderOpts` / `XPathRenderOpts` struct carrying `IncludeSensitive bool`. Redacted values are emitted as `TypedValue{StringVal: "***"}`.

**Admin bypass.** `include_sensitive: bool` added to `GetIntentRequest` and `BlameConfigRequest` in `sdc-protos`. Trust model: any caller that can reach the gRPC port (unchanged from today). Stronger auth (mTLS cert role) can be layered later without touching the redaction logic.

**Phase-2 composition.** A future intent path-slice approach (sensitive paths stored per-intent in cache) unions with this static baseline at render time. The extension point is `RenderOpts.SensitivePathSet map[string]bool`; render functions extend the condition to `ops.IsSensitive(e) || opts.SensitivePathSet[ops.KeyPrunedPath(e)]`. No structural changes to the ops or processor layers are needed.

**Out of scope (this ADR).** `WatchDeviations` redaction — deferred; streaming flag threading is a distinct problem. Encrypt-at-rest — separate KMS concern. Per-instance sensitivity — list keys are stripped; all instances of a leaf type are treated uniformly.

## Amendments

**2026-09-22 — Phase-2 path markers, always-union, SensitiveRender, index split.**

The original decision (schema baseline via `sdcio-ext:sensitive`) is unchanged. These amendments record what landed on top of that baseline:

- **Path markers (Phase-2 done).** Intent-declared `sensitive_paths` contribute to a datastore-wide **Live Sensitive Path Index**. `types.ShouldRedact` is the single predicate: schema sensitivity OR path-marker membership, unless the caller opts in via include-sensitive.
- **Always-union northbound.** GetIntent (named + Running), BlameConfig, and WatchDeviations always pass the live index — own-scope GetIntent snapshots are retired. Cross-intent markers redact on northbound output.
- **WatchDeviations redacts.** Streaming deviations resolve redaction in the ops layer via `RenderOptsNorthbound(false, liveIndex)`; the earlier “deferred” note for WatchDeviations is superseded for value masking (streaming flag threading for an admin bypass remains separate if needed).
- **Index split.** `SensitivePathIndex` is live-only (`Set`/`Delete`/`Contains`). Immutable `SensitivePaths` is the snapshot checker for tests/helpers. Presence-only `Add` is removed.
- **Opaque `SensitiveRender`.** Redaction context (include flag + checker) lives in `ops.SensitiveRender`, embedded in `RenderOpts`. Callers use `RenderOptsNorthbound` / `RenderOptsRevealAll`; public `IncludeSensitive` / `SensitivePathSet` fields are gone. Formatters call `ShouldRedact` / `TypedValue` / `String` and do not invent `***`.

**2026-10-09 — Single `pkg/tree/sensitive` package; options structs at the datastore boundary (PR #493 review).**

- **One redaction mechanism.** The sentinel, the live index and the render context moved from `pkg/tree/types` and `pkg/tree/ops` into `pkg/tree/sensitive` (`Render`, `PathIndex`, `PathChecker`, `RedactedStringValue`, `RedactedTypedValue`). `api` cannot import `ops`, which is why `LeafVariants.GetDeviations` previously took a pre-computed `shouldRedact bool` and duplicated the redact closure. It now takes a `sensitive.Render` and resolves redaction against its own parent entry. `types.ShouldRedact` is folded into `Render.ShouldRedact`, which takes an entry-like `sensitive.Leaf` (satisfied by `api.Entry`).
- **`SensitivePaths` removed.** The immutable snapshot checker had no production users; tests use `PathIndex`. `PathChecker` stays as the seam, and a nil checker still means "no markers".
- **Not done: redacting in the leaf accessor.** Making `LeafEntry` accessors return already-redacted values (so a consumer cannot forget to redact) was considered and deferred; it would change the accessor surface just migrated in #492.
- **Datastore RPC options.** `GetIntent` and `BlameConfig` on `*Datastore` take `GetIntentOpts` / `BlameConfigOpts` in `package datastore` (shared embedded `NorthboundSensitivityOpts` with `IncludeSensitive`, matching proto). Server maps gRPC requests into these structs; tests call the same API. Empty `IntentName` is rejected; zero `IncludeSensitive` means redact. The datastore builds `sensitive.Render` via `RenderOptsNorthbound` internally — callers never assemble it.

**2026-10-26 — Transaction rollback restores path markers (PR #493 review).**

- **Snapshot at capture.** When `TransactionSet` records `TransactionIntentOld`, it snapshots path markers from the Live Sensitive Path Index (`PathIndex.PathsFor`) at the same moment as updates and explicit deletes — before this transaction writes cache or index.
- **Rollback applies the snapshot.** `TransactionCancel` and confirm-timeout rollback replay that snapshot into `intent.sensitive_paths` and `PathIndex.Set`, so marker-only redaction is not cleared by a rolled-back transaction.
- **Source of truth at capture.** The index is authoritative for the snapshot; cache and index are kept aligned on successful `IntentModify`. Replace transactions are out of scope (different rollback model).

