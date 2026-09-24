# 01 — Inventory legacy #442 / #480 commits

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** A committed mapping of every meaningful commit on the old Cisco (#442) and SONiC (#480) stacks into exactly one of: **device-profile base**, **generic fix to `main`**, **SONiC NOS PR**, or **Cisco NOS PR**. Include notes for any short-lived cherry-pick bridges onto the base branch and precondition links between generic-fix PRs and NOS PRs.

**Blocked by:** None — can start immediately.

**Status:** done

**Deliverable:** [legacy-commit-inventory.md](../legacy-commit-inventory.md)

- [x] Each legacy commit is classified with a single destination bucket (no orphans)
- [x] Generic vs NOS-only vs base-only boundaries match the parallel-stacks spec
- [x] Mapping is checked in (or linked from the base PR body) so stack splits and rebases do not rely on tribal knowledge
- [x] Protos dependency is recorded as the agreed **sdc-protos#120 branch commit** (pseudo-version in `go.mod`), not a separate “merge protos first” step
