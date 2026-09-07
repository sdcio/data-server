# 04 — Post follow-up comment on issue #483 / PR #442 with live-lab results

**What to build:** A comment on [issue #483](https://github.com/sdcio/data-server/issues/483) and/or [PR #442](https://github.com/sdcio/data-server/pull/442) (a first comment addressed to `@ipgst` was already posted at PR #442 — see https://github.com/sdcio/data-server/pull/442#issuecomment-5570748414 — this ticket is the *follow-up* once 01-03 land, not a duplicate of that one) summarizing: what was verified against a local lab, what passed exactly as the reporter's own repro predicted (module-anchored `permodule` mechanism works as-is), what failed exactly as the reporter found (`PROTO`/`JSON` categorically rejected — now caught at config-load instead of at runtime), and the residual open question (tested on XR 7.10.1, reporter is on 26.2.1 — invite them to confirm once they can retest).

**Blocked by:** 01, 02, 03 — the comment should describe landed work, not a plan.

**Status:** ready-for-agent

- [ ] Draft comment referencing the specific probe results (see spec.md's Further Notes for the full list) in the same style as the earlier PR #442 comment (concrete `gnmic` commands + results, not just prose).
- [ ] Link the new ADR (ticket 03) and the merged/landed commit(s) (tickets 01/02).
- [ ] Explicitly ask the reporter to retest `device-profile: cisco-ios-xr` + `encoding: JSON_IETF` end-to-end against their own XR 26.2.1 hardware once they can pull an updated image, given the version gap noted above.
- [ ] Mention the config-server-side prerequisite ([sdcio/config-server#484](https://github.com/sdcio/config-server/pull/484), currently open/unmerged) needed to actually set `device-profile` via the `TargetConnectionProfile` CRD, since the reporter's original manifests didn't reference it at all.
- [ ] Post via `gh pr comment 442 -R sdcio/data-server` (and/or `gh issue comment 483 -R sdcio/data-server`, per whichever thread is more active by the time this ticket is worked).

## Comments
