# Index: configserver-cache-namespace-scoping

Ticket list for this effort, in dependency order. Each ticket's `Status:` line lives in its own file — this index is a read-only summary, not the source of truth for status.

| # | Ticket | Blocked by | Status |
|---|--------|-----------|--------|
| [01](01-namespace-name-split-helper.md) | Standalone namespace/name split helper | None | done |
| [02](02-wire-split-into-configservercache.md) | `ConfigServerCache` derives namespace/name per call, drops fixed `namespace` field | 01 | ready-for-agent |
| [03](03-delete-cacheconfig-namespace-field.md) | Delete `CacheConfig.Namespace` config field and its validation | 02 | ready-for-agent |

Not ticketed: `pkg/cache/CONTEXT.md`'s "Datastore name" / "Target namespace" / "Target name" glossary entries (spec implementation decision #6) were already added during the `/grilling` session that produced the spec — no further action needed.

## Working the frontier

1. Scan the table above (re-reading each ticket's own `Status:` line, since that's authoritative, not this table) for tickets that are `ready-for-agent` **and** unblocked (every ticket listed in its "Blocked by" is `Status: done`).
2. Among those, pick the lowest-numbered one — that's the frontier ticket.
3. On completion, set that ticket's `Status:` line to `done`, update its row in the table above to match, and append a one-line pointer under a `## Comments` heading in the ticket file noting what landed (commit/branch if applicable).
4. If no ticket is both `ready-for-agent` and unblocked, report that the frontier is empty instead of guessing.
