# pkg/datastore

## Transaction

A `types.Transaction` (`pkg/datastore/types/transaction.go`) bundles everything needed to apply
(and, if needed, revert) a single `TransactionSet` call:

- `newIntents` — the intents the caller asked to set/delete, keyed by intent name.
- `oldIntents` — for every touched intent, its content *before* this transaction (empty if the
  intent didn't exist yet). Always populated, regardless of whether the transaction is a replace.
- `oldRunning` — the pre-transaction Running snapshot (see below). Only meaningful for replace
  transactions.
- `replace` — set when the transaction is a full-device replace (`nil` otherwise).

`GetRollbackTransaction()` builds the `Transaction` that undoes the original one: it always
replays `oldIntents` as new intents, and additionally sets `.replace` from `oldRunning`, but only
when the original transaction had `.replace` set. This is what makes a timed-out/canceled replace
transaction actually revert the device, instead of being a silent no-op (a plain rollback of an
empty `oldIntents` set does nothing for a replace, since a replace by itself never touches
`oldIntents`).

## Replace transaction

A replace transaction (`transaction.GetReplace() != nil`) discards the whole device configuration
and re-applies it from a single intent's content (`replaceIntent`, `pkg/datastore/transaction_rpc.go`).
It needs a base to replace *from*: the Running snapshot as it stood immediately before the replace
was applied.

`replaceIntent` no longer fetches that snapshot itself. Instead, the shared orchestration method
`Datastore.replaceThenMerge` fetches it once, up front (`d.cacheClient.IntentGet(ctx,
consts.RunningIntentName)`), before either the replace or the merge (`lowlevelTransactionSet`)
phase runs, and passes it down to `replaceIntent`. `replaceThenMerge` is used by both
`Datastore.TransactionSet` and `DatastoreRollbackAdapter.TransactionRollback`, so a rollback
transaction's own `.replace` (see above) is honored too, not silently dropped.

## `oldRunning`

`oldRunning` is the flattened (`PathAndUpdate`) form of the Running snapshot fetched at the top of
`replaceThenMerge`, captured on the transaction *before* the replace phase (if any) mutates
anything. It exists purely to make replace-transaction rollbacks possible: `GetRollbackTransaction`
uses it as the rollback's `.replace` payload when the original transaction was a replace.

This is intentionally a one-shot snapshot taken at transaction-start time, not a live notion of
"Running is currently fresh" — the transaction owns that copy for its own lifetime (including any
later rollback of it) and never re-fetches it.
