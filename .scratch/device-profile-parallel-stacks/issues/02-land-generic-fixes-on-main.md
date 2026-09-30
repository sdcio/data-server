# 02 — Land generic correctness fixes on `main`

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** Tree, XPath, converter, datastore concurrency, yang-parser, subscribe target naming, and other **non–profile-specific** changes from the inventory land on `main` as separate green PRs. NOS stack branches rebase onto `main` and link any still-open fix PRs as preconditions in their bodies.

**Blocked by:** 01 — Inventory legacy #442 / #480 commits

**Status:** open PR [#505](https://github.com/sdcio/data-server/pull/505) to `main` (not merged yet)

- [x] Every inventory item tagged “generic fix to `main`” is either merged to `main` or has an open PR with a clear owner
- [x] No SONiC-only or Cisco-only encoder behavior is smuggled in under generic-fix PRs
- [x] Stack branches (`device-profile-base`, SONiC, Cisco) document which fix PRs they depend on before merge — use precondition table in [legacy-commit-inventory.md](../legacy-commit-inventory.md#precondition-links-ticket-02--05) until **02** is on `main`
- [x] Default integration path is merge to `main` then rebase stacks; cherry-picks to base are rare, documented, and called out in PR text

## Delivered on `ticket-02-generic-fixes`

Single PR to `main` (parent spec’s “separate PRs” intent batched for review; can split later if reviewers prefer).

| Inventory **main** SHA | Theme |
|------------------------|--------|
| `c368539`, `b3aecc4` | Datastore `sbi` mutex |
| `e16aaa7` | Import config key slice clone (merged comment with existing clone on `main`) |
| `4275945`, `4ea1b4f`, `1417054` | List must / key-level / `NavigateSdcpbPath` + yang-parser bump |
| `57f1c7d`, `a06a69e` | gNMI subscribe `TargetName` (+ call sites on current `main`) |
| `cbb179d`, `10fa760` | RFC7951 self-wrap unwrap (scoped) + must xpath error context |
| `95b8408` | **Main parts only** — converter unwrap + must errors; **not** `GnmiSetPlan` / materialize (ticket **03**) |

**Explicitly not in this PR:** `sdc-protos#120` pin, materialize/setplan, SONiC/Cisco encoders or profile enablement.

## Suggested PR body snippet for **05** / **06**

> Preconditions: merge [ticket 02 PR](…) to `main` (or cherry-pick per [inventory bridges](../legacy-commit-inventory.md#cherry-pick-bridges-short-lived-only) until rebase).
