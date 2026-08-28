# 04 — Wire sonic dispatch into `materialize.BuildPlan`

**What to build:** An operator with `device-profile: sonic` configured on a gNMI Target gets a correctly-encoded `GnmiSetPlan` end to end when a Set is applied — `materialize.BuildPlan` recognizes the sonic profile and routes to the new encoder, with no NOS-specific branching leaking into `gnmi.go`, `target.New`, or other generic infrastructure. This is the tracer bullet that makes tickets 01 and 03 demoable together as one working path.

**Blocked by:** 01 (device-profile config field, `IsSonic()` predicate, JSON_IETF-only validation), 03 (sonic encoder package)

**Status:** done

- [x] `materialize.BuildPlan` gains a branch: `sbi.Type == "gnmi" && sbi.IsSonic()` calls the new encoder's `Encode(...)`, mirroring exactly how the existing `sbi.IsCiscoIOSXR()` branch calls `permodule.Encode(...)`.
- [x] No fallback to the generic single-root gNMI plan for non-`JSON_IETF` encodings under this profile (this is enforced by ticket 01's config-time validation, not a runtime fallback/error branch).
- [x] `target.New` remains unaffected — device-profile selection happens only inside `materialize.BuildPlan`.
- [x] The sonic dispatch condition and any profile-specific logic live entirely inside `materialize.BuildPlan` and the encoder package; `gnmi.go` remains pure transport with no sonic-specific code.
- [x] Routing unit tests (same style as the existing `materialize_test.go` Cisco IOS-XR routing tests): a sonic-profile SBI with JSON_IETF encoding routes to the new encoder; non-sonic profiles are unaffected by the change.

## Comments

- Landed on branch `sonic-device-profile`, commit `90c8796`: added `sbi.IsSonic()` dispatch branch in `materialize.BuildPlan` with two routing tests in `materialize_test.go`.
