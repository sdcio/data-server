# Device-profile parallel stacks — ticket index

**Spec:** [`.scratch/open/10-device-profile-parallel-stacks-spec.md`](../open/10-device-profile-parallel-stacks-spec.md)  
**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

Delivery: **device-profile base** → parallel **SONiC** and **Cisco** NOS PRs (not stacked on each other). Protos: pin **sdc-protos#120** branch commit in base `go.mod` (no separate “merge protos first” ticket).

## Tickets

| # | Ticket | Blocked by | Status |
|---|--------|------------|--------|
| 01 | [Inventory legacy #442 / #480 commits](issues/01-inventory-legacy-442-480-commits.md) | — | done → [legacy-commit-inventory.md](legacy-commit-inventory.md) |
| 02 | [Land generic correctness fixes on `main`](issues/02-land-generic-fixes-on-main.md) | 01 | open PR [#505](https://github.com/sdcio/data-server/pull/505) |
| 03 | [Device-profile base PR](issues/03-device-profile-base-pr.md) | 01; 02 merges to `main` (inventory) | open PR [#506](https://github.com/sdcio/data-server/pull/506) |
| 04 | [config-server: `deviceProfile` on base stack](issues/04-config-server-device-profile-on-base.md) | 03 | done on branch → `device-profile-base` (config-server) |
| 05 | [SONiC NOS PR](issues/05-sonic-nos-pr.md) | 03 | open PR [#507](https://github.com/sdcio/data-server/pull/507) |
| 06 | [Cisco IOS-XR NOS PR (draft)](issues/06-cisco-ios-xr-nos-pr-draft.md) | 03 | open draft PR [#508](https://github.com/sdcio/data-server/pull/508) |
| 07 | [Supersede legacy PRs #442 and #480](issues/07-supersede-legacy-prs-442-480.md) | 03, 05, 06 | done → [supersession-record.md](supersession-record.md) |

## Dependency graph

```mermaid
flowchart TD
  T01[01 Inventory]
  T02[02 Generic fixes to main]
  T03[03 Base PR]
  T04[04 config-server]
  T05[05 SONiC]
  T06[06 Cisco draft]
  T07[07 Close 442/480]
  T01 --> T02
  T01 --> T03
  T02 -.->|if required| T03
  T03 --> T04
  T03 --> T05
  T03 --> T06
  T03 --> T07
  T05 --> T07
  T06 --> T07
```

## Frontier

Tickets whose blockers are satisfied now:

**Merge order:** [#505](https://github.com/sdcio/data-server/pull/505) → [#506](https://github.com/sdcio/data-server/pull/506) → [#507](https://github.com/sdcio/data-server/pull/507) / [#508](https://github.com/sdcio/data-server/pull/508) when ready (parallel NOS stacks).

**In flight:** **04** — config-server branch `device-profile-base` → PR to `main` (pairs with data-server [#506](https://github.com/sdcio/data-server/pull/506)).

Ticket **07** complete: legacy [#442](https://github.com/sdcio/data-server/pull/442) and [#480](https://github.com/sdcio/data-server/pull/480) closed; see [supersession-record.md](supersession-record.md).

## Git branches (from spec)

| Branch | PR |
|--------|-----|
| `ticket-02-generic-fixes` | [#505](https://github.com/sdcio/data-server/pull/505) → `main` |
| `device-profile-base` | [#506](https://github.com/sdcio/data-server/pull/506) → `ticket-02-generic-fixes` |
| `sonic-device-profile` | [#507](https://github.com/sdcio/data-server/pull/507) → `device-profile-base` |
| `cisco-ios-xr-gnmi` | [#508](https://github.com/sdcio/data-server/pull/508) → `device-profile-base` (draft) |

## Out of scope (see spec)

Synced-gate #501, stacking SONiC on Cisco, enabling both profiles in one NOS PR, config-server E2E that assumes working NOS before the matching data-server PR merges.
