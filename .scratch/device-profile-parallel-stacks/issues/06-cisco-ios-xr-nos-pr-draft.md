# 06 — Cisco IOS-XR NOS PR (`cisco-ios-xr-gnmi`, draft)

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** Parallel NOS stack off device-profile base (not stacked on SONiC): module-anchored Set encoder, JSON_IETF-only validation when enabled, materialize dispatch for Cisco, and tests from legacy #442 / product anchor #483. **Profile enablement for `cisco-ios-xr` ships in the same PR** intended to merge—not a follow-up flip. PR stays **draft** until IOS-XR gNMI Set behavior is acceptable in lab/CI.

**Blocked by:** 03 — Device-profile base PR

**Status:** ready-for-agent

- [ ] Only `cisco-ios-xr` is enabled in this PR; `sonic` remains governed by ticket 05
- [ ] Granular per-module Set work for #483 lives on this stack, not on the old #442→#480 chain
- [ ] Plain JSON rejected for Cisco profile when enabled; encoder and materialize tests mirror prior permodule coverage
- [ ] Draft status held until lab-ready; no merge pressure for broken Cisco support
