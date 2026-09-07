# 02 — Narrow `materialize.BuildPlan` dispatch to `JSON_IETF`; fix doc comments

**What to build:** `materialize.BuildPlan`'s `cisco-ios-xr` branch only routes to `permodule.Encode` for `JSON_IETF`; `JSON` (now unreachable via valid config per ticket 01, but `BuildPlan` may still be called directly in tests) falls through to the generic single-root-update path instead, matching how `PROTO` already falls through today. Doc comments across the file are corrected to stop claiming `JSON`/`PROTO` support that doesn't exist for this profile.

**Blocked by:** None — independent of ticket 01, can be done in either order or in parallel.

**Status:** done

- [ ] In `pkg/datastore/target/materialize/materialize.go`'s `BuildPlan`, narrow the `cisco-ios-xr` inner switch from `case gnmi.Encoding_JSON, gnmi.Encoding_JSON_IETF:` to `case gnmi.Encoding_JSON_IETF:` only.
- [ ] Update `BuildPlan`'s doc comment bullet list: the line `"gnmi" with DeviceProfile "cisco-ios-xr" + json/json_ietf → GnmiSetPlan (per YANG module via permodule)"` becomes `"gnmi" with DeviceProfile "cisco-ios-xr" + json_ietf → GnmiSetPlan (per YANG module via permodule); json/proto rejected at config-load (see pkg/config)`.
- [ ] Update `buildGnmiPlan`'s doc comment if it references `cisco-ios-xr`'s JSON support (check for any stale cross-references while in the file).
- [ ] Do not touch `permodule`'s package (`pkg/datastore/target/gnmi/permodule/`) — its mechanism is verified correct and out of scope for this ticket.
- [ ] Tests in `pkg/datastore/target/materialize/materialize_test.go`: add/update a routing test confirming `cisco-ios-xr` + `JSON` no longer routes to `permodule.Encode` (falls through to the generic single-root path instead — assert on the produced plan shape, e.g. path is the root/empty path rather than a module-scoped one). Keep the existing `JSON_IETF`-routes-to-`permodule` assertion passing.
- [ ] `go test ./pkg/datastore/target/materialize/...` green.

## Comments

- Landed on branch `ciscoiosxrd2`: narrowed `BuildPlan`'s `cisco-ios-xr` inner switch to `case gnmi.Encoding_JSON_IETF:` only (plain `JSON` now falls through to the generic single-root path, matching `PROTO`), updated the `BuildPlan` doc comment bullet, and added `TestBuildPlan_CiscoIOSXR_JSON_GenericPlan` in `materialize_test.go` asserting the generic-path shape. `go test ./...` green.
