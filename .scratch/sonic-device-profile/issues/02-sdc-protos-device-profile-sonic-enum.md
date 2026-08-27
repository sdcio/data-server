# 02 — sdc-protos: add `DEVICE_PROFILE_SONIC` to the `DeviceProfile` enum

**What to build:** The `DeviceProfile` enum in `sdc-protos`' `data.proto` gains `DEVICE_PROFILE_SONIC = 2`, landed on the existing open `deviceprofile` branch (base `main`, the same branch that already carries `DEVICE_PROFILE_CISCO_IOS_XR = 1` from [sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)) — not opened as a separate branch. This is what lets the sonic profile eventually be exposed on the Target CR/gRPC API, matching how the Cisco IOS-XR profile is exposed today.

**Blocked by:** None — can start immediately, in parallel with the data-server-side tickets. It is a cross-repo dependency: ticket 05 (gRPC/Target-CR exposure in data-server) cannot land until this enum value is available on that branch and data-server's `go.mod` can be bumped to a commit that includes it.

**Status:** ready-for-agent

- [ ] `data.proto`'s `enum DeviceProfile` gains `DEVICE_PROFILE_SONIC = 2` on the `deviceprofile` branch.
- [ ] Generated Go code for the enum is regenerated/committed on that branch.
- [ ] Existing `DEVICE_PROFILE_CISCO_IOS_XR = 1` value and any other enum values on the branch are unaffected.
