# PRD: Mutate-on-Write Leaf Deviation Handling

## Problem Statement

Some devices mutate a value on write before it can ever be read back identically — the canonical example is a secret pushed in plaintext that the device hashes and stores. The next `Get`/`Sync` cycle returns the hashed value, which today is compared directly against the intent's plaintext value in `LeafVariants.GetDeviations` (`pkg/tree/api/leaf_variants.go:538`). Because that comparison never becomes true again, the leaf permanently reports a `NOT_APPLIED` **Deviation**, even though nothing is actually wrong — the device applied the write correctly and simply stores it differently than it was sent.

Compounding this, `TransactionSet`'s writeback of Running (`pkg/datastore/transaction_rpc.go:395`, "OPTIMISTIC WRITEBACK TO RUNNING") writes the pushed **intent** value into Running immediately after a successful apply, not a value read back from the device. For a mutate-on-write leaf this is doubly wrong for one cycle: Running briefly holds the plaintext, then the next `Sync` corrects it to the hashed value, and only then does the permanent false-positive `NOT_APPLIED` above kick in.

Source: [sdcio/.github#36](https://github.com/sdcio/.github/issues/36).

## Solution

Introduce a **Confirmed applied value** and a **mutate-on-write leaf** classification (see `docs/CONTEXT.md` for full definitions). Summary of decisions reached in the issue discussion:

1. **No new reapply trigger.** The trigger for pushing a new value to the device stays exactly as it is today: a caller submitting a `Transaction` whose merged intent value actually changed for that path. No background reconciliation loop is introduced off the back of `NOT_APPLIED` detection — `DeviationManager` continues to only report, never act.
2. **New per-`(Owner, path)` state: confirmed applied value.** After a successful apply, for paths classified as mutate-on-write, perform a targeted read-back of just those paths and store the result as the confirmed applied value for that `(Owner, path)`. This read also fixes the optimistic-writeback bug for these leaves specifically — Running is populated from the read-back, not the pushed plaintext.
3. **Deviation comparison override.** For a mutate-on-write leaf, `GetDeviations` compares Running against the confirmed applied value instead of the winning **Intent**'s literal value. If no confirmed applied value exists yet for the current highest-precedence owner, comparison falls back to today's Intent-value comparison (so a leaf that was never actually written still reports `NOT_APPLIED` correctly).
4. **Invalidation is implicit.** When the intent value changes and gets re-pushed, the confirmed applied value for the old value is simply superseded by a fresh read-back after the new push — no separate invalidation step needed.
5. **Dual, unioned classification source**, mirroring the existing **Sensitive leaf** model:
   - Schema-level YANG extension, parallel to `sdcio-ext:sensitive` ([ADR 0003](../../adr/0003-yang-extension-schema-sensitive-baseline.md)).
   - Intent-level per-path marker submitted at `TransactionSet` time, parallel to `sensitive_paths` ([PR #460](https://github.com/sdcio/data-server/pull/460)).
   - Either source is sufficient — union (OR), not intersection. This matches the merged-view union direction from [ADR 0004](../../adr/0004-scoped-sensitive-path-union-per-operation.md), since deviation computation is itself a merged view (unlike single-intent `GetIntent`, which uses only that intent's own markers for sensitivity).
   - Rationale for union over "only the currently-winning owner's declaration": ownership of a leaf can change hands between intents over time; the classification decision (is this leaf mutate-on-write at all) should stay stable regardless of who currently owns it, while the confirmed-applied-value baseline naturally resets and repopulates per-owner via the fallback in point 3. The two concerns don't need the same scoping rule.
6. **Explicitly rejected: a general-purpose "ignore deviation" flag.** `Non-revertive intent` already covers "report drift honestly, don't auto-correct." A generic suppress-reporting escape hatch was considered and rejected as dangerous — it would hide genuine drift, not just the narrow mutate-on-write case this issue is about.

## Open Questions

Not yet resolved in discussion — needed before implementation:

- Exact schema extension name (`mutate-on-write` is a working name only).
- Behavior when the post-write read-back fails, times out, or the device settles asynchronously (e.g. requires a reboot/commit before the mutated value is visible).
- Leaf-list / list-instance handling — the sensitive-leaf precedent strips list keys and treats all instances of a leaf type uniformly; unclear whether mutate-on-write needs the same simplification or per-instance confirmed values.
- Storage mechanism for confirmed applied value (new field on `LeafEntry` vs. a new cache-backed structure) and its lifecycle across datastore restart.
- Interaction with `TransactionCancel` / rollback / timeout — does a confirmed applied value get rolled back along with the intent it belongs to?
- Interaction with `Sync`'s normal periodic read path — does the targeted read-back after apply get superseded by (or race with) the next regular `Sync` cycle?

## Out of Scope

- General-purpose deviation suppression / generic "ignore this deviation" flag (explicitly rejected — see decision 6).
- Any automatic background reapply/reconciliation loop triggered by detected deviations (explicitly rejected — see decision 1).

## Further Notes

The optimistic-writeback behavior in `writeBackSyncTree` is a pre-existing, separately-noteworthy issue: it writes the pushed intent value into Running for *every* leaf, not just mutate-on-write ones, ahead of any real confirmation from the device. This PRD only commits to fixing it for mutate-on-write leaves (as a side effect of the targeted read-back in decision 2); whether the general optimistic writeback should also change is out of scope here and would need its own discussion.
