# 05 — Expose `DeviceProfileSonic` on the gRPC/Target-CR layer

**What to build:** A maintainer setting up the Target API/CRD can (eventually) select the sonic device-profile the same way the Cisco IOS-XR profile is selectable today — the `DeviceProfile` enum value from `sdc-protos` round-trips correctly through data-server's gRPC layer into local config and back, so the profile is not left as a config-file-only value.

**Blocked by:** 01 (`DeviceProfileSonic` constant must exist in `pkg/config`), 02 (the `DEVICE_PROFILE_SONIC` enum value must be available on the `sdc-protos` `deviceprofile` branch)

**Status:** ready-for-agent

- [ ] data-server's `go.mod` is bumped to a `sdc-protos` commit on the `deviceprofile` branch that includes `DEVICE_PROFILE_SONIC = 2`.
- [ ] `sdcpbDeviceProfileToConfig` gains a case mapping `sdcpb.DeviceProfile_DEVICE_PROFILE_SONIC` → `config.DeviceProfileSonic`, matching the existing Cisco IOS-XR mapping.
- [ ] `configDeviceProfileToSdcpb` gains the corresponding reverse-mapping case.
- [ ] Existing Cisco IOS-XR and default/generic mapping behavior is unaffected.
- [ ] Unit/coverage for the new mapping cases, consistent with how the existing Cisco IOS-XR mapping is (or should be) covered.
