# 07 — Supersede legacy PRs #442 and #480

**Parent:** [GitHub #504](https://github.com/sdcio/data-server/issues/504)

**What to build:** After replacement stack PRs exist (base, SONiC, Cisco draft), close #442 and #480 with superseded-by comments pointing at the three new PRs so discussion and links redirect cleanly. Do not close before replacements are open.

**Blocked by:** 03 — Device-profile base PR; 05 — SONiC NOS PR; 06 — Cisco IOS-XR NOS PR (draft)

**Status:** done — [supersession-record.md](../supersession-record.md)

- [x] Base, SONiC, and Cisco replacement PRs are open on the correct branches and bases ([#506](https://github.com/sdcio/data-server/pull/506), [#507](https://github.com/sdcio/data-server/pull/507), [#508](https://github.com/sdcio/data-server/pull/508); generic fixes [#505](https://github.com/sdcio/data-server/pull/505))
- [x] #442 and #480 closed with explicit supersession pointers to the new PRs
- [x] Parent issue #504 and spec remain the canonical plan reference
