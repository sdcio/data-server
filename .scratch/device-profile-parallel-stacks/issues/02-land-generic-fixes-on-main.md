# 02 — Land generic correctness fixes on `main`

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** Tree, XPath, converter, datastore concurrency, yang-parser, subscribe target naming, and other **non–profile-specific** changes from the inventory land on `main` as separate green PRs. NOS stack branches rebase onto `main` and link any still-open fix PRs as preconditions in their bodies.

**Blocked by:** 01 — Inventory legacy #442 / #480 commits

**Status:** ready-for-agent

- [ ] Every inventory item tagged “generic fix to `main`” is either merged to `main` or has an open PR with a clear owner
- [ ] No SONiC-only or Cisco-only encoder behavior is smuggled in under generic-fix PRs
- [ ] Stack branches (`device-profile-base`, SONiC, Cisco) document which fix PRs they depend on before merge
- [ ] Default integration path is merge to `main` then rebase stacks; cherry-picks to base are rare, documented, and called out in PR text
