# 01 — Sonic device-profile config field and closed-set validation

**What to build:** An operator can set `device-profile: sonic` on a Target's gNMI SBI config. Config validation accepts `sonic` as a known device-profile value (alongside the existing none/cisco-ios-xr values), and specifically rejects the SBI at config-load time if `device-profile: sonic` is combined with any `GnmiOptions.Encoding` other than `JSON_IETF`. This gives operators a fast, clear config-time error instead of a confusing runtime Set failure.

**Blocked by:** None — can start immediately.

**Status:** done

- [x] New `DeviceProfileSonic DeviceProfile = "sonic"` constant added alongside the existing `DeviceProfileNone`/`DeviceProfileCiscoIOSXR` constants.
- [x] ~~New `(*SBI).IsSonic() bool` predicate added, mirroring the existing `IsCiscoIOSXR()` predicate.~~ _Landed then removed during ticket 04 refactor — see below._
- [x] Closed-set device-profile validation in `SBI.validateSetDefaults()` accepts `"sonic"` as a valid value (unknown values still rejected).
- [x] When `device-profile == "sonic"`, validation rejects any `GnmiOptions.Encoding` other than `JSON_IETF` at config-load time.
- [x] Unit tests: `sonic` + `JSON_IETF` accepted; `sonic` + `JSON` rejected; `sonic` + `PROTO` rejected; unaffected profiles/encodings unchanged. _(The `IsSonic()` predicate test was removed when the predicate was deleted.)_

## Comments

- Landed on branch `sonic-device-profile` (based off local `ciscoiosxrd2`), commit adding `DeviceProfileSonic`/`IsSonic()`/encoding validation + tests in `pkg/config/datastore.go` and `pkg/config/sbi_device_profile_test.go`.
- During ticket 04: `IsSonic()` and `IsCiscoIOSXR()` were identified as middle-man predicates and removed (commit `b5980bd`). Dispatch now compares `sbi.DeviceProfile` directly against the exported `DeviceProfileSonic`/`DeviceProfileCiscoIOSXR` constants. `SBITypeGnmi`/`SBITypeNetconf`/`SBITypeNoop` were also exported at the same time (commit `c7aa62e`).
