# 05 — (Optional safety net) `ProcessErrors` doesn't fail unrelated Configs for LoadAll-only owners

**Spec:** `.scratch/last-applied-snapshot-write-at-apply/spec.md` (see User Story 14, "Optional narrow unit" under Level 1 testing)

**What to build:** When a validation error's owning Intent was only present via `LoadAllButRunningIntents` (not one of this RPC's own intents), `ProcessErrors` must not mark an unrelated Config in the same transaction as failed ("unknown intent"). This is explicitly a safety net, not the primary fix — ticket 03 fixing the ghost-intent write-timing bug should make this scenario rare-to-nonexistent on its own.

**Blocked by:** 03 (needs correct apply-time last-applied semantics in place first; this is a narrower net on top, not a substitute)

- [ ] Unit test: a validation error owned by a LoadAll-only Intent does not fail an unrelated Config's processing in the same transaction
- [ ] No change to `ProcessErrors`'s behavior for errors owned by an Intent that *is* part of this RPC's own intent set
- [ ] This ticket may be dropped without blocking merge if ticket 03 alone eliminates the scenario in practice (confirm via the ghost+customer-delete integration scenario in ticket 06 before deciding to drop)
