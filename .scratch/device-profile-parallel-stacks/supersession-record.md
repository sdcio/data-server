# Legacy PR supersession (ticket 07)

**Date:** 2026-09-24  
**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

## Closed legacy PRs

| Legacy | Branch | Closed |
|--------|--------|--------|
| [#442](https://github.com/sdcio/data-server/pull/442) | `ciscoiosxrd2` → `main` | yes — superseded |
| [#480](https://github.com/sdcio/data-server/pull/480) | `sonic-device-profile` → `ciscoiosxrd2` | yes — superseded |

## Replacement stack (data-server)

| Role | PR | Head → base |
|------|-----|-------------|
| Generic fixes to `main` (ticket 02) | [#505](https://github.com/sdcio/data-server/pull/505) | `ticket-02-generic-fixes` → `main` |
| Device-profile base (ticket 03) | [#506](https://github.com/sdcio/data-server/pull/506) | `device-profile-base` → `ticket-02-generic-fixes` |
| SONiC NOS (ticket 05) | [#507](https://github.com/sdcio/data-server/pull/507) | `sonic-device-profile` → `device-profile-base` |
| Cisco IOS-XR NOS draft (ticket 06) | [#508](https://github.com/sdcio/data-server/pull/508) | `cisco-ios-xr-gnmi` → `device-profile-base` |

Canonical plan remains [#504](https://github.com/sdcio/data-server/issues/504) and [spec](../open/10-device-profile-parallel-stacks-spec.md).
