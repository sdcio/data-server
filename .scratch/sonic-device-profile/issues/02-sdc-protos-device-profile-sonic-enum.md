# 02 — sdc-protos: add `DEVICE_PROFILE_SONIC` to the `DeviceProfile` enum

**What to build:** The `DeviceProfile` enum in `sdc-protos`' `data.proto` gains `DEVICE_PROFILE_SONIC = 2`, landed on the existing open `deviceprofile` branch (base `main`, the same branch that already carries `DEVICE_PROFILE_CISCO_IOS_XR = 1` from [sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120)) — not opened as a separate branch. This is what lets the sonic profile eventually be exposed on the Target CR/gRPC API, matching how the Cisco IOS-XR profile is exposed today.

**Blocked by:** None — can start immediately, in parallel with the data-server-side tickets. It is a cross-repo dependency: ticket 05 (gRPC/Target-CR exposure in data-server) cannot land until this enum value is available on that branch and data-server's `go.mod` can be bumped to a commit that includes it.

**Status:** done

- [x] `data.proto`'s `enum DeviceProfile` gains `DEVICE_PROFILE_SONIC = 2` on the `deviceprofile` branch.
- [x] Generated Go code for the enum is regenerated/committed on that branch.
- [x] Existing `DEVICE_PROFILE_CISCO_IOS_XR = 1` value and any other enum values on the branch are unaffected.

## Comments

- Landed on `sdc-protos` branch `deviceprofile`, commit [`40ed0bc`](https://github.com/sdcio/sdc-protos/commit/40ed0bc26a71263302a5b24b741ab67db4fac01f) (`feat(deviceprofile): add DEVICE_PROFILE_SONIC enum value`), stacked on [sdc-protos#120](https://github.com/sdcio/sdc-protos/pull/120). Pushed to `origin/deviceprofile`.
- Downstream: [05](05-expose-sonic-profile-on-grpc-target-cr.md) bumps `go.mod` to this commit and wires the gRPC mapping.
