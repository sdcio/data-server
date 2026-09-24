# 05 — SONiC NOS PR (`sonic-device-profile`)

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** Parallel NOS stack off device-profile base: enable **`sonic` only**—lift base-layer rejection for that profile, JSON_IETF-only validation, parent-bound Set encoding with translib origin semantics, materialize dispatch to the SONiC encoder, Get request shaping required by translib (e.g. ALL) at the sanctioned target-construction seam, and tests from legacy #480 SONiC scope. PR base is `device-profile-base` until base merges, then rebase onto `main`. Precondition links to any generic fix PRs from ticket 02 still in flight.

**Blocked by:** 03 — Device-profile base PR

**Status:** done on branch `sonic-device-profile` (worktree `/home/mava/projects/data-server-sonic`)

- [x] Only `sonic` is enabled; `cisco-ios-xr` remains on the base stub
- [x] Set and Get behavior is isolated to SONiC packages and enabled dispatch arms
- [x] Config, server mapping, materialize routing, and encoder unit tests cover enablement and routing without live devices
- [ ] PR body lists precondition links to open generic-fix PRs on `main` if any remain
