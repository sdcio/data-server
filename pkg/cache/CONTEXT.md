# Cache

Owns the `Client` abstraction data-server uses to read and write per-target intent
and device-state data, independent of which backend (`local`, `config-server`)
actually stores it.

## Language

**Intent**:
A named, prioritized piece of northbound-authored config, owned by whichever
system is the sole writer for the active `Cache.Type` (config-server, or
data-server itself under `local`). Read through the `Client.InstanceIntent*`
family.

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
