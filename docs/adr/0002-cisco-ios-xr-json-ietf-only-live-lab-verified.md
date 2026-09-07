# Cisco IOS-XR `permodule` mechanism verified against real hardware; profile restricted to `JSON_IETF` only

ADR [0001](0001-ios-xr-granular-gnmi-json-from-api-entry.md)'s module-anchored `permodule` gNMI encoding for the `cisco-ios-xr` device profile was verified against a real Cisco XRd instance and found correct as designed — no defect in `permodule`, its dispatch, or ADR 0001's mechanism. Separately, and as a distinct decision, the `cisco-ios-xr` profile is now restricted to `encoding: JSON_IETF` only: `PROTO` and plain `JSON` are rejected at config-load time rather than allowed to fail at reconcile time against the device.

## Why

[Issue #483](https://github.com/sdcio/data-server/issues/483) reported declarative `Config` writes failing against real Cisco IOS XRd 26.2.1 when using `encoding: JSON_IETF` (data-server's *generic*, non-profiled single-root-update path, not `permodule`) and `encoding: PROTO` (native scalar `TypedValue`s). Because ADR 0001's `permodule` mechanism had never been tested against real hardware before merge, it wasn't clear whether the reporter's failures indicated a defect in `permodule` itself or were specific to the *un-profiled* paths they had actually exercised.

We reproduced the reporter's scenarios end-to-end against a local `containerlab` Cisco XRd instance (`clab-cisco-ixr01`, image `registry.srlinux.dev/pub/cisco_8201-32fh_214:7.10.1`, `cisco_c8000` containerlab kind, gNMI on port `57400`) and additionally probed `permodule`'s actual wire shape directly, to settle both questions independently.

## Verification: ADR 0001's `permodule` mechanism (not superseded)

All probes were run against the `MgmtEth0/RP0/CPU0/0` interface `description` leaf and, for the multi-module and module-root-delete cases, the `hostname` leaf — both already present in the reporter's own minimal schema.

| Shape probed | Result |
|---|---|
| Per-leaf `json_ietf_val` (baseline, matches reporter's own working repro) | Pass |
| Module-root container `json_ietf_val`, `origin` = module name unprefixed (exact `permodule` shape) | Pass |
| `origin: cisco_native` + module-prefixed path element | Pass |
| Two-module single `SetRequest` | Pass |
| Module-root delete | Pass (clean, no side effects) |
| Delete + update on the same module in one `SetRequest` (replace semantics) | Pass |

**Conclusion: ADR 0001 is not superseded.** Its module-anchored mechanism — `Path.origin` set to the literal YANG module name, module name not prefixed onto the path element, one `Update` per module-root container carrying the whole subtree as `json_ietf_val`, multiple modules batched into a single `SetRequest`, replace expressed as delete+update for the same module in one `SetRequest` — works correctly as-is against real XRd hardware. This ADR only adds the `JSON_IETF`-only restriction below as a separate decision, plus this verification record; it does not change or contradict ADR 0001's design.

## Decision: restrict `cisco-ios-xr` + `gnmi` to `encoding: JSON_IETF`

| Shape probed | Result |
|---|---|
| Plain `json_val` (non-IETF), per-leaf | Fail — `"not supported val type: 10"` |
| Plain `json_val` (non-IETF), module-root | Fail — `"not supported val type: 10"` |
| `PROTO` `string_val`, per-leaf | Fail — `"not supported val type: 1"` |
| `PROTO` `uint_val`, per-leaf | Fail — `"not supported val type: 3"` |

Plain `JSON` is rejected regardless of granularity (both per-leaf and module-root fail identically); `PROTO` was probed at per-leaf granularity only, but fails with the same `"not supported val type: N"` class of error for every native scalar `TypedValue` tried, independent of leaf/path correctness. Together these indicate XRd's native-YANG `Set` endpoint accepts `json_ietf_val` only, full stop — not a `permodule` defect or a granularity problem specific to one shape. This exactly matches the reporter's own `PROTO` failure and explains why their `JSON_IETF` failure (via the generic, un-profiled path) also failed: the generic path's single whole-datastore-root update is a different problem (no module scoping at all), but is downstream of the same "IETF JSON, module-scoped" requirement `permodule` already satisfies.

## Considered options

**Rejected — leave `PROTO`/`JSON` reachable, let `Set` fail at reconcile time:** This is the status quo the reporter hit. `materialize.BuildPlan`'s `cisco-ios-xr` branch already only handled `JSON`/`JSON_IETF` via `permodule`, falling through to the generic path for `PROTO`; a `cisco-ios-xr` + `JSON` config would reach `permodule` and produce the now-proven-broken `json_val` wire shape, and `cisco-ios-xr` + `PROTO` would reach the generic path's unshaped, also-broken output. Both produce a confusing device-side error (`"not supported val type: N"`) at `Set` time instead of a clear error at config-load time.

**Accepted — reject at config-load time:** `SBI.validateSetDefaults()` now rejects `cisco-ios-xr` + `gnmi` + any encoding other than `JSON_IETF` (landed in [pkg/config/datastore.go](../../pkg/config/datastore.go)). `materialize.BuildPlan`'s `cisco-ios-xr` inner switch is narrowed to `case gnmi.Encoding_JSON_IETF:` only (landed in [pkg/datastore/target/materialize/materialize.go](../../pkg/datastore/target/materialize/materialize.go)), so plain `JSON` — unreachable via valid config, but still reachable if `BuildPlan` is called directly, e.g. in tests — falls through to the generic path rather than into `permodule` with a wire shape known to fail on real hardware, mirroring how `PROTO` already fell through. This mirrors the `sonic` device profile's own `JSON_IETF`-only config-validation approach. No change was made to `permodule`'s `Encode`, `moduleRootPath`, `encodeMerge`, `encodeReplace`, or any origin/path-construction logic — that mechanism is verified correct and untouched.

## Residual risk: XR version gap

Verification was performed against XR **7.10.1** (`cisco_c8000`/8201-32FH containerlab image). Issue #483's reporter is running XR **26.2.1** — a substantially newer release train. This verification is strong evidence that the `permodule` mechanism and the `JSON_IETF`-only restriction are correct, but it is **not** a substitute for confirmation on the reporter's actual hardware/software version. This is tracked as an open question for the reporter in the issue/PR follow-up, not closed by this ADR.
