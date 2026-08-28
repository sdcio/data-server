# 05 — Expose `DeviceProfileSonic` on the gRPC/Target-CR layer

**What to build:** A maintainer setting up the Target API/CRD can (eventually) select the sonic device-profile the same way the Cisco IOS-XR profile is selectable today — the `DeviceProfile` enum value from `sdc-protos` round-trips correctly through data-server's gRPC layer into local config and back, so the profile is not left as a config-file-only value.

**Blocked by:** 01 (`DeviceProfileSonic` constant must exist in `pkg/config`), 02 (the `DEVICE_PROFILE_SONIC` enum value must be available on the `sdc-protos` `deviceprofile` branch — see [02](02-sdc-protos-device-profile-sonic-enum.md#comments))

**Status:** done

**Cross-repo pin (from ticket 02):** Bump `github.com/sdcio/sdc-protos` in `go.mod` from `67240812f373` to [`40ed0bc`](https://github.com/sdcio/sdc-protos/commit/40ed0bc26a71263302a5b24b741ab67db4fac01f) on branch `deviceprofile` ([sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)). When this ticket lands, cross-reference back in ticket 02's Comments with the data-server commit/branch.

- [x] data-server's `go.mod` is bumped to a `sdc-protos` commit on the `deviceprofile` branch that includes `DEVICE_PROFILE_SONIC = 2`.
- [x] `sdcpbDeviceProfileToConfig` gains a case mapping `sdcpb.DeviceProfile_DEVICE_PROFILE_SONIC` → `config.DeviceProfileSonic`, matching the existing Cisco IOS-XR mapping.
- [x] `configDeviceProfileToSdcpb` gains the corresponding reverse-mapping case.
- [x] Existing Cisco IOS-XR and default/generic mapping behavior is unaffected.
- [x] Unit/coverage for the new mapping cases, consistent with how the existing Cisco IOS-XR mapping is (or should be) covered.

## Comments

- Landed on branch `sonic-device-profile`: bumped `sdc-protos` to `v0.0.55-0.20260828064538-40ed0bc26a71`; added `DEVICE_PROFILE_SONIC` cases to both mapping functions in `pkg/server/datastore.go`; added table-driven coverage in `pkg/server/datastore_device_profile_test.go`.
