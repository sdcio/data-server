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
