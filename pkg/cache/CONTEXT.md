# Cache

Owns the `Client` abstraction data-server uses to read and write per-target intent
and device-state data, independent of which backend (`local`, `config-server`)
actually stores it.

## Language

**Intent**:
A named, prioritized piece of northbound-authored config — the *desired*
value. Authorship (who decides what an Intent's content should be) belongs
to whichever system is the sole writer for the active `Cache.Type`
(config-server, or data-server itself under `local`). Read through the
`Client.InstanceIntent*` family. _Avoid_: using "Intent" when you mean
last-applied — under `Cache.Type: config-server` these are two different
values with two different owners (see **Last-applied** below).

**Last-applied**:
The value of a named Intent that this datastore last successfully pushed
southbound — updated at the same moment `IntentModify`/`IntentDelete` run
inside `TransactionSet`'s apply loop, for every `Cache.Type`, not gated on
`TransactionConfirm`. Under `Cache.Type: local` this has always been true by
construction (disk write happens at that exact call). Under
`Cache.Type: config-server`, data-server writes this back into config-server
so `Client.InstanceIntent*` reads (backed by `TargetSnapshot.Spec.Configs`)
never lag behind what was actually applied — a deleted Intent must stop
being last-applied at delete-apply time, not at some later, best-effort
snapshot refresh, or the next `LoadAllButRunningIntents` can rehydrate config
that was meant to be gone. _Avoid_: "confirmed" or "acknowledged" (collide
with the separate `TransactionConfirm` RPC step) and "expected state" (says
nothing about whether it's desired-or-applied).

**Running**:
The synced, on-device configuration state — produced and consumed entirely
in-process (`ops.TreeExport` / `ImportConfigAdapter`), never authored
northbound. Not an `Intent`: it is exempt from the `Client.InstanceIntent*`
family (own accessors, own storage, excluded from `InstanceIntentGetAll`)
under every backend, because it was never config-server's data to begin with.
_Avoid_: calling it an intent, "the running intent" as a `Client`-level type.

**ImportConfigAdapter**:
The mechanical, backend-agnostic shape `Client`'s read-side methods return:
"data ready to hand to `Tree.ImportConfig`." Every `Client.*Get`/`*GetAll`
method returns this shape — including `InstanceRunningGet` — regardless of
whether the underlying value is an `Intent` or `Running`, and regardless of
how many concrete representations exist behind it for a given backend.
