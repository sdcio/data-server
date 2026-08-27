# 01 — Sonic device-profile config field and closed-set validation

**What to build:** An operator can set `device-profile: sonic` on a Target's gNMI SBI config. Config validation accepts `sonic` as a known device-profile value (alongside the existing none/cisco-ios-xr values), and specifically rejects the SBI at config-load time if `device-profile: sonic` is combined with any `GnmiOptions.Encoding` other than `JSON_IETF`. This gives operators a fast, clear config-time error instead of a confusing runtime Set failure.

**Blocked by:** None — can start immediately.

**Status:** done

- [x] New `DeviceProfileSonic DeviceProfile = "sonic"` constant added alongside the existing `DeviceProfileNone`/`DeviceProfileCiscoIOSXR` constants.
- [x] New `(*SBI).IsSonic() bool` predicate added, mirroring the existing `IsCiscoIOSXR()` predicate.
- [x] Closed-set device-profile validation in `SBI.validateSetDefaults()` accepts `"sonic"` as a valid value (unknown values still rejected).
- [x] When `device-profile == "sonic"`, validation rejects any `GnmiOptions.Encoding` other than `JSON_IETF` at config-load time.
- [x] Unit tests (same style as the existing Cisco IOS-XR device-profile tests): `sonic` + `JSON_IETF` accepted; `sonic` + `JSON` rejected; `sonic` + `PROTO` rejected; unaffected profiles/encodings unchanged; `IsSonic()` predicate behaves correctly for sonic/none/other profiles.

## Comments

- Landed on branch `sonic-device-profile` (based off local `ciscoiosxrd2`), commit adding `DeviceProfileSonic`/`IsSonic()`/encoding validation + tests in `pkg/config/datastore.go` and `pkg/config/sbi_device_profile_test.go`.
