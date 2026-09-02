# 05 — (Optional safety net) `ProcessErrors` doesn't fail unrelated Configs for LoadAll-only owners

Status: done

**Spec:** `.scratch/last-applied-snapshot-write-at-apply/spec.md` (see User Story 14, "Optional narrow unit" under Level 1 testing)

**What to build:** When a validation error's owning Intent was only present via `LoadAllButRunningIntents` (not one of this RPC's own intents), `ProcessErrors` must not mark an unrelated Config in the same transaction as failed ("unknown intent"). This is explicitly a safety net, not the primary fix — ticket 03 fixing the ghost-intent write-timing bug should make this scenario rare-to-nonexistent on its own.

**Blocked by:** 03 (needs correct apply-time last-applied semantics in place first; this is a narrower net on top, not a substitute)

- [x] Unit test: a validation error owned by a LoadAll-only Intent does not fail an unrelated Config's processing in the same transaction
- [x] No change to `ProcessErrors`'s behavior for errors owned by an Intent that *is* part of this RPC's own intent set
- [x] This ticket may be dropped without blocking merge if ticket 03 alone eliminates the scenario in practice (confirm via the ghost+customer-delete integration scenario in ticket 06 before deciding to drop) — kept rather than dropped, since ticket 06 (integration-level ghost+customer-delete proof) has not landed yet in this worktree

## Comments

Implemented on `data-server` branch `config-server-cache-backend-race` (worktree at `/home/mava/projects/data-server-worktrees/config-server-cache-backend-race`).

There is no function literally named `ProcessErrors` in data-server (that name lives on the config-server reconciler side); the equivalent data-server concept is `lowlevelTransactionSet`'s validation-result handling in `pkg/datastore/transaction_rpc.go` — the block that populates `result.Intents` from `validationResult` and then calls `validationResult.HasErrors()` to decide whether to abort the transaction with `ErrValidation`.

**Root cause confirmed:** `ValidationResults.HasErrors()` is unscoped — any intent key with errors (including a ghost intent rehydrated by `LoadAllButRunningIntents` that is not part of this RPC's own intents) aborts the *entire* transaction before `applyIntent`/cache-writeback ever runs, even when the RPC's own intents (e.g. an unrelated `customer` delete) have no errors of their own. This is the CI hang described in ADR 0003 and the spec's ghost+customer-delete scenario.

**Fix:** `LoadAllButRunningIntents`'s return value (previously discarded) is now compared against `transaction.GetNewIntents()` to compute the set of intent names that are LoadAll-only (loaded but not part of this RPC). A new `ValidationResults.HasErrorsExcludingOwners(excludeOwners map[string]struct{})` method (`pkg/tree/types/validation_result.go`) is used in place of the bare `HasErrors()` call at the abort-decision site only — `result.Intents` population, warnings, and the `replaceIntent` path's own `HasErrors()` call (a different, out-of-scope code path) are unchanged. Errors owned by an RPC-owned intent still abort the transaction exactly as before (`TestTransactionSet_ValidationError_OwnedByRPCIntent_StillFails`).

New tests:
- `TestTransactionSet_LoadAllOnlyValidationError_DoesNotBlockUnrelatedIntent` (`pkg/datastore/transaction_rpc_test.go`) — ghost `intent1` with a dangling network-instance leafref, RPC deletes unrelated `customer`; asserts `TransactionSet` succeeds and `IntentDelete("customer", ...)` is called.
- `TestTransactionSet_ValidationError_OwnedByRPCIntent_StillFails` (same file) — control case, same broken leafref content submitted directly as the RPC's own intent; asserts the response still carries the error and apply is never reached (no `Set()` expectation registered on the mock target).
- `TestValidationResults_HasErrorsExcludingOwners` (`pkg/tree/types/validation_result_test.go`) — table test at the narrower seam covering empty results, excluded-only errors, non-excluded errors, both together, and nil exclude set (parity with plain `HasErrors()`).

Not dropped: ticket 06 (the integration-level ghost+customer-delete proof mentioned as the drop criterion) has not landed in this worktree, so this safety net stays in place per the ticket's own guidance not to drop it without that confirmation first.

**Standards + Spec review follow-ups applied:**
- Dropped the duplicated loop shape between `HasErrors()` and `HasErrorsExcludingOwners()` — `HasErrors()` now delegates as `return v.HasErrorsExcludingOwners(nil)`.
- Reverted the shared `buildFixtureIntent` test helper's `SkipValidation` back to `false` (its original behavior for every existing caller) and added a narrowly-scoped `buildFixtureIntentAllowInvalid` variant, used only by the new ghost-intent fixture, so no other test's fixture validation is weakened.
- Added an assertion that the excluded ghost intent's own error is still reported in `resp.Intents["intent1"]` even though it no longer blocks the transaction — pins down that exclusion changes only the abort decision, not response shaping.
