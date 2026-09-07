Status: ready-for-agent

# Cisco IOS-XR device-profile: restrict to `JSON_IETF`, reject `PROTO`/`JSON` at config-load

## Problem Statement

[sdcio/data-server#442](https://github.com/sdcio/data-server/pull/442) added the `cisco-ios-xr` device-profile (module-anchored, per-module `json`/`json_ietf` gNMI `Set` encoding via `permodule`) but it was never tested against real hardware before merge-readiness. [Issue #483](https://github.com/sdcio/data-server/issues/483) reports declarative `Config` writes failing against real Cisco IOS XRd 26.2.1 when using `encoding: JSON_IETF` (data-server's *generic*, non-profiled path — a single whole-datastore-root update) and `encoding: PROTO` (native scalar `TypedValue`s, e.g. `string_val`), with XRd rejecting both.

We reproduced the reporter's scenarios end-to-end against a local `containerlab` Cisco XRd instance (image family `cisco_c8000`/8201-32FH, XR 7.10.1 — an older release train than the reporter's 26.2.1, noted as a residual gap) and additionally probed exactly what `permodule`'s current wire shape produces. Results:

- `permodule`'s existing encoding — `Path.Origin = "<literal YANG module name>"`, module name **not** prefixed onto the path element, one `Update` per module-root container carrying the whole subtree as `json_ietf_val`, multiple modules batched into one `SetRequest`, replace expressed as delete+update for the same module in one `SetRequest` — **works correctly as-is**. No defect found in `permodule`, its dispatch, or ADR 0001's mechanism.
- Plain `JSON` (`json_val`, non-IETF) is **categorically rejected** by XRd regardless of granularity or path shape: `"not supported val type: 10"`.
- `PROTO`'s native scalar `TypedValue`s (`string_val`, `uint_val`, etc.) are **categorically rejected** regardless of leaf/path correctness: `"not supported val type: N"` for every non-JSON-IETF value type tried. This is not a granularity or origin-field problem — XRd's native-YANG `Set` endpoint appears to accept `json_ietf_val` only, full stop.

So the actual gap is narrow: `materialize.BuildPlan`/`pkg/config` currently let an operator configure `cisco-ios-xr` with `encoding: PROTO` (silently falls through to the unshaped generic gNMI path — exactly the reporter's failing PROTO reproduction) or `encoding: JSON` (routes into `permodule` but produces a wire format XRd rejects outright). Both should be caught at config-load time instead of producing a confusing runtime `Set` failure, mirroring the pattern already established for `sonic` (`ciscoiosxrd2`'s direct descendant branch), which restricts to `JSON_IETF`-only for its own, unrelated, translib-specific reasons.

## Solution

Restrict the `cisco-ios-xr` device-profile to `encoding: JSON_IETF` only, for `type: gnmi` SBIs:

- Config-load-time validation rejects `cisco-ios-xr` + `gnmi` + any encoding other than `JSON_IETF` (currently `PROTO` is accepted and silently unshaped; `JSON` is accepted and silently produces a wire format that fails on real hardware — both become load-time errors).
- `materialize.BuildPlan`'s `cisco-ios-xr` dispatch branch is narrowed to `JSON_IETF` only (drops the now-provably-broken `JSON` case); anything else no longer reaches `permodule` and instead falls through to the generic path only as defensive dead code (config validation is the primary guard, matching the `sonic` profile's own documented approach).
- No change to `permodule`'s actual encoding logic, `gnmi.go`, or `target.New` — the verified-correct mechanism is left untouched.
- Doc comments (`DeviceProfileCiscoIOSXR`'s doc comment, `materialize.BuildPlan`'s doc comment) updated to state the `JSON_IETF`-only restriction instead of claiming `JSON`/`JSON_IETF` are both supported.
- A new ADR records the live-lab verification and the encoding restriction decision (existing ADR 0001 is not superseded — its module-anchored mechanism was empirically confirmed correct, not contradicted).
- Reply posted to issue #483 / PR #442 summarizing the live-lab results and the resulting fix, including the residual XR-version gap (tested on 7.10.1, reporter is on 26.2.1) as an open caveat for the reporter to confirm against their own hardware once released.

## User Stories

1. As an operator configuring a Target with `device-profile: cisco-ios-xr` and `encoding: PROTO`, I want the SBI config to be rejected at load time, so that I get a fast, clear error instead of `Config` CRs silently failing at reconcile time with a misleading device-side error message.
2. As an operator configuring a Target with `device-profile: cisco-ios-xr` and `encoding: JSON` (non-IETF), I want the SBI config to be rejected at load time for the same reason — this shape is confirmed to fail against real XRd hardware, not just untested.
3. As an operator configuring a Target with `device-profile: cisco-ios-xr` and `encoding: JSON_IETF`, I want the config to continue to be accepted and to continue producing the same `permodule`-encoded `Set` requests as today — this path is verified working and must not change.
4. As a maintainer reading `pkg/config`'s `DeviceProfileCiscoIOSXR` doc comment or `materialize.BuildPlan`'s doc comment, I want them to accurately state that only `JSON_IETF` is supported for this profile, so the comments don't claim `PROTO`/`JSON` support that doesn't exist.
5. As a maintainer working on the `sonic` device-profile (a direct descendant branch of `ciscoiosxrd2`), I want this fix expressed as a config-validation check in the same style/location as `sonic`'s own `JSON_IETF`-only restriction (a sibling `if sbi.DeviceProfile == DeviceProfileCiscoIOSXR && ...` check next to the existing closed-set switch in `SBI.validateSetDefaults()`, using direct constant comparison rather than the `IsCiscoIOSXR()` predicate), so that rebasing `sonic-device-profile` onto the updated `ciscoiosxrd2` stays a low-conflict, additive merge rather than requiring re-reconciliation of two different validation patterns. (Note: `sonic-device-profile` ticket 04 already removes the `IsCiscoIOSXR()`/`IsSonic()` predicates as middle-man wrappers per the repo's no-tight-coupling rule — this work should not reintroduce a new call site for that predicate.)
6. As a future reader of ADR 0001, I want a follow-up ADR documenting that the module-anchored mechanism was verified against real XRd hardware (with the specific probes and results), and that the JSON_IETF-only restriction is a separate, distinct decision from the module-anchoring one — so the two aren't conflated when someone revisits this later.
7. As the issue #483 reporter, I want a clear summary of what was verified, what changed, and what remains an open question specific to their XR 26.2.1 (vs. our tested 7.10.1) environment, so they know what to re-verify rather than assuming everything is settled.

## Implementation Decisions

### Config validation (`pkg/config/datastore.go`)

- Add a check in `SBI.validateSetDefaults()`, sibling to the existing `sonic` JSON_IETF-only check, of the shape: `if s.DeviceProfile == DeviceProfileCiscoIOSXR && s.Type == sbiGNMI && !strings.EqualFold(s.GnmiOptions.Encoding, "JSON_IETF") { return fmt.Errorf(...) }`. Use direct `DeviceProfile` comparison, not the `IsCiscoIOSXR()` predicate (see User Story 5 — avoids adding a new call site for a predicate that `sonic`'s own ticket 04 already plans to remove).
- `netconf` + `cisco-ios-xr` is unaffected by this change (no encoding concept there; already accepted per existing test `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRNetconfIsAccepted`).
- Update `DeviceProfileCiscoIOSXR`'s doc comment to state the `JSON_IETF`-only restriction plainly (current comment claims "JSON / JSON_IETF granular path encoding; other encodings use the generic gNMI plan builder" — the "other encodings fall back to generic" claim becomes false for `gnmi`+`cisco-ios-xr` once this validation lands).

### Dispatch (`pkg/datastore/target/materialize/materialize.go`)

- Narrow the `cisco-ios-xr` branch's inner switch to `case gnmi.Encoding_JSON_IETF:` only (drop `gnmi.Encoding_JSON`), so a `JSON`-configured `cisco-ios-xr` SBI (which config validation now prevents from ever existing, but `BuildPlan` may still be called directly in unit tests) falls through to the generic single-root-update path rather than routing into `permodule` with a wire shape known to fail on real hardware. This mirrors how `PROTO` already falls through today, and matches `sonic`'s stated approach of config-time validation as the primary guard with `BuildPlan` behavior as a secondary, non-authoritative detail.
- Update `BuildPlan`'s doc comment bullet list: `"gnmi" with DeviceProfile "cisco-ios-xr" + json/json_ietf → GnmiSetPlan (per YANG module via permodule)"` becomes `"gnmi" with DeviceProfile "cisco-ios-xr" + json_ietf → GnmiSetPlan (per YANG module via permodule); json/proto rejected at config-load`.
- No change to `permodule`'s `Encode`, `moduleRootPath`, `encodeMerge`, `encodeReplace`, or any of its origin/path-construction logic — verified correct against real hardware, left untouched.

### ADR

- New ADR (next available number in `docs/adr/`) documenting: (a) the live-lab verification of ADR 0001's module-anchored mechanism (what was tested, against what hardware/version, and the specific results — origin-as-module-name + unprefixed element + module-root scoping + multi-module-single-`SetRequest` + delete-then-update-replace all confirmed working), and (b) the `JSON_IETF`-only restriction as a distinct decision, with the categorical-rejection evidence for `JSON` and `PROTO` (`"not supported val type: N"` for every non-JSON-IETF value type tried, independent of path/leaf granularity).
- Explicitly note in the new ADR that the verification hardware (XR 7.10.1 via a local `containerlab cisco_c8000`/8201-32FH image) is an older release train than issue #483's reporter (XR 26.2.1 XRd), so the verification is strong evidence but not a substitute for the reporter confirming against their own train.

### Issue/PR reply

- Post a follow-up comment on #483 and/or #442 summarizing: what was tested, what passed, what failed exactly as the reporter found (`PROTO`/`JSON` categorically rejected), the fix being made (config-load rejection, doc corrections), and the residual open question (XR 26.2.1 vs. tested 7.10.1) inviting the reporter to confirm on their own hardware once the fix lands.

## Testing Decisions

Two seams, both extending existing test files/styles already in the repo — no new seams:

1. **Config validation** (`pkg/config/sbi_device_profile_test.go`): flip the existing `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIProtoIsAccepted` to assert rejection instead (rename accordingly), and add a new `..._CiscoIOSXRGNMIPlainJSONIsRejected` case. Keep `..._CiscoIOSXRGNMIJSONIsAccepted` (JSON_IETF) and `..._CiscoIOSXRNetconfIsAccepted` as-is (still passing, unaffected). Style/table shape should mirror the equivalent `sonic` tests in the same file once that branch rebases, so the two profiles' test blocks read as obvious siblings.
2. **Dispatch** (`pkg/datastore/target/materialize/materialize_test.go`): update/add a routing test confirming `cisco-ios-xr` + `JSON` no longer routes to `permodule.Encode` (falls through to the generic single-root path instead), alongside the existing `JSON_IETF`-routes-to-`permodule` assertion.

Only test external behavior (the validation error / the produced `SouthboundSetPlan` shape), not internal helpers, per existing repo test style. No `permodule` test changes needed — its behavior is unchanged and already covered by `pkg/datastore/target/gnmi/permodule/encode_test.go`.

No further live-lab verification is required for this specific PR (the verification described in the Problem Statement was already performed manually as part of scoping this spec); a follow-up round on the reporter's own XR 26.2.1 hardware happens via their response to the issue/PR comment, not as part of this PR's completion bar.

## Out of Scope

- Any change to `permodule`'s encoding mechanism (`Path.Origin`, module-root path construction, per-module batching, replace-as-delete-then-update) — verified correct, not touched.
- Any change to `sonic`'s encoder, config validation, or dispatch — this work stays entirely within the `cisco-ios-xr` profile's own branch of the config-validation/dispatch code. The `sonic-device-profile` branch will rebase onto this work later; no attempt is made here to pre-emptively merge or unify the two profiles' validation logic beyond matching sibling-`if`-check style.
- Removing/refactoring the existing `IsCiscoIOSXR()` predicate — left as-is; `sonic-device-profile` ticket 04 already owns that removal.
- Re-verifying against XR 26.2.1 specifically, or against the reporter's exact `first-boot`/discovery/schema setup — out of scope for this PR; tracked as the open question for the issue/PR reply.
- Any change to Get/Subscribe/sync behavior for `cisco-ios-xr` — untouched by both the issue and this fix; only `Set` was ever in question.
- The `sdc-protos`/`config-server` side of exposing `device-profile` on the Target CR — already handled by [sdcio/config-server#484](https://github.com/sdcio/config-server/pull/484) (currently open, unmerged), tracked separately and not part of this spec.

## Further Notes

- This work happens directly on the local `ciscoiosxrd2` branch (PR #442's actual branch — a dedicated worktree was created at `/home/mava/projects/data-server-worktrees/ciscoiosxrd2` for it, leaving the in-flight `sonic-device-profile` checkout untouched), since `sonic-device-profile` branches directly off `ciscoiosxrd2` per its own spec's "Further Notes" — landing this fix here means `sonic` inherits it on its next rebase rather than needing a duplicate/parallel fix.
- Live-lab verification detail (for the ADR and issue/PR reply): local `containerlab` instance `clab-cisco-ixr01`, image `registry.srlinux.dev/pub/cisco_8201-32fh_214:7.10.1` (`cisco_c8000` containerlab kind), gNMI reachable on port `57400` (the lab's `first-boot.cfg` requested port `9339` but that portion of the config failed to load at boot — an unrelated lab-tooling issue, not a data-server or XR gNMI behavior finding).
- Probes run (all against the `MgmtEth0/RP0/CPU0/0` interface `description` leaf and, for the multi-module/module-root-delete cases, the `hostname` leaf, both already present in the reporter's own minimal schema): per-leaf json_ietf (baseline, matches reporter's working repro) — pass; module-root-container json_ietf, origin=module unprefixed (exact `permodule` shape) — pass; `origin: cisco_native` + module-prefixed element — pass; plain `json_val`, both per-leaf and module-root — fail (`"not supported val type: 10"`); `PROTO` `string_val`/`uint_val`, per-leaf — fail (`"not supported val type: 1"`/`"3"`); two-module single `SetRequest` — pass; module-root delete — clean, no side effects; delete+update same module in one `SetRequest` (replace semantics) — pass.
