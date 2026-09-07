# 01 — Reject `cisco-ios-xr` + `PROTO`/`JSON` at config-load time

**What to build:** `SBI.validateSetDefaults()` in `pkg/config/datastore.go` rejects a `gnmi`-type SBI configured with `device-profile: cisco-ios-xr` and any `GnmiOptions.Encoding` other than `JSON_IETF`. This replaces the current silent-fallthrough (`PROTO`) / silent-wrong-wire-format (`JSON`) behavior with a fast, clear config-load error — both shapes are confirmed to fail against real XRd hardware (see spec.md's Problem Statement for the probe results).

**Blocked by:** None — can start immediately.

**Status:** done

- [ ] Add a check in `SBI.validateSetDefaults()`, sibling to the existing device-profile closed-set switch, of the shape: when `s.DeviceProfile == DeviceProfileCiscoIOSXR` and `s.Type == sbiGNMI`, reject unless `strings.EqualFold(s.GnmiOptions.Encoding, "JSON_IETF")`. Use direct `DeviceProfile` comparison — do **not** introduce a new call site for the `IsCiscoIOSXR()` predicate (see spec.md User Story 5: that predicate is slated for removal by the `sonic-device-profile` branch's own ticket 04, and a new call site here would just be more code for that rebase to touch).
- [ ] `netconf` + `cisco-ios-xr` must remain accepted (no encoding concept there) — don't scope the new check to `s.Type == sbiGNMI` incorrectly and accidentally reject netconf.
- [ ] Update `DeviceProfileCiscoIOSXR`'s doc comment in `pkg/config/datastore.go` to state the `JSON_IETF`-only restriction (drop the "other encodings use the generic gNMI plan builder" claim for `gnmi` — that's no longer true once this validation lands; `netconf` acceptance-without-shaping is unaffected and can stay documented as-is).
- [ ] Tests in `pkg/config/sbi_device_profile_test.go`:
  - Flip `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIProtoIsAccepted` to assert rejection; rename to `..._CiscoIOSXRGNMIProtoIsRejected`.
  - Add `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIPlainJSONIsRejected` (encoding `"json"`, expect error).
  - Leave `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRGNMIJSONIsAccepted` (encoding `"json_ietf"`) and `TestSBI_validateSetDefaults_DeviceProfile_CiscoIOSXRNetconfIsAccepted` passing, unchanged.
- [ ] `go test ./pkg/config/...` green.

## Comments

- Landed on branch `ciscoiosxrd2`: added the `JSON_IETF`-only check in `SBI.validateSetDefaults()` (`pkg/config/datastore.go`), updated `DeviceProfileCiscoIOSXR`'s doc comment, and updated/added tests in `pkg/config/sbi_device_profile_test.go` (`..._CiscoIOSXRGNMIProtoIsRejected` renamed/flipped, `..._CiscoIOSXRGNMIPlainJSONIsRejected` added). `go test ./...` green.
