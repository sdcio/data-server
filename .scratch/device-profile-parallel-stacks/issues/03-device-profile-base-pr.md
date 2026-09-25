# 03 — Device-profile base PR (`device-profile-base` → `main`)

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** Shared southbound plumbing only: materialization between intent apply and target Set, plan types (gNMI/NETCONF), targets consuming plans instead of profile encoding inside transport drivers, config and CreateDataStore exposure of `device_profile` with **both** proto enum values known but **not enabled**, and dispatch arms for SONiC and Cisco that **fail closed** with a clear error. Generic/empty profile behavior matches today. `go.mod` pins **sdc-protos** to the agreed **sdc-protos#120 branch commit** (pseudo-version / `replace`, same pattern as legacy stack PRs)—no prerequisite merge of #120 into protos `main`. ADR updates for materialization and dispatch seams. Full `go test ./...` green with both NOS profiles disabled.

**Blocked by:** 01 — Inventory legacy #442 / #480 commits; 02 — Land generic fixes on `main` (only for items the inventory marks as required for base to build or review)

**Status:** done on branch `device-profile-base`

- [x] Config validation and CreateDataStore reject `sonic` and `cisco-ios-xr` with deterministic **not enabled** errors; generic profile accepted
- [x] gRPC enum round-trip for all three values without enabling NOS behavior
- [x] `BuildPlan` (or equivalent): generic gNMI/NETCONF unchanged; disabled profile arms error without calling NOS packages (absent on this branch)
- [x] No NOS encoder packages in this PR; no vendor profile checks scattered outside sanctioned dispatch entry points
- [x] Protos pin in `go.mod` matches the #120 commit that includes **both** `DEVICE_PROFILE_SONIC` and `DEVICE_PROFILE_CISCO_IOS_XR`
- [x] Boundary tests at config validation, gRPC mapping, and materialize dispatch mirror the spec’s primary test seams
- [x] ADR narrative extended for materialization and per-profile Get/Set dispatch points
