# Index: cache-client-capability-split

Ticket list for this effort, in dependency order. Each ticket's `Status:` line lives in its own file — this index is a read-only summary, not the source of truth for status.

| # | Ticket | Blocked by | Status |
|---|--------|-----------|--------|
| [01](issues/01-split-client-interfaces.md) | Split `cache.Client`/`CacheClientBound` into capability interfaces | None | done |
| [02](issues/02-noop-intent-writer-for-configserver.md) | `noopIntentWriter` for config-server, composed at `createCacheClient` | 01 | done |
| [03](issues/03-narrow-intent-reader-consumers.md) | Narrow `forEachIntent`/`populateSensitivePathIndex` to `IntentReader` | 01 | done |
| [04](issues/04-local-decode-dedup-and-importer-relocation.md) | `LocalCache` decode dedup + relocate `documentImporter`/`mergeConfigBlobs` | None | done |

## "tdd next" protocol

When the user says "tdd next":

1. Scan the table above (re-reading each ticket's own `Status:` line, since that's authoritative, not this table) for tickets that are `ready-for-agent` **and** unblocked (every ticket listed in its "Blocked by" is `Status: done`).
2. Among those, pick the lowest-numbered one — that's the frontier ticket. If two are unblocked (e.g. 01 and 04, or 02 and 03 once 01 is done), lowest number wins.
3. Run `/tdd` against that ticket file (not the whole spec) — treat its `## What to build` as the seam description and its checklist as the acceptance criteria to drive the red -> green loop.
4. On completion, set that ticket's `Status:` line to `done`, update its row in the table above to match, and append a one-line pointer under a `## Comments` heading in the ticket file noting what landed (commit/branch if applicable).
5. If no ticket is both `ready-for-agent` and unblocked, report that the frontier is empty (either everything is done, or something upstream is still blocked) instead of guessing.
