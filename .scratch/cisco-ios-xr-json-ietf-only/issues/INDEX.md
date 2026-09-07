# Index: cisco-ios-xr-json-ietf-only

Ticket list for this effort, in dependency order. Each ticket's `Status:` line lives in its own file — this index is a read-only summary, not the source of truth for status.

## Where to work

Do this work in the worktree at `/home/mava/projects/data-server-worktrees/ciscoiosxrd2`, on branch
`ciscoiosxrd2` (PR [#442](https://github.com/sdcio/data-server/pull/442)'s actual branch). Do not use
`/home/mava/projects/data-server` directly — that checkout is on `sonic-device-profile`, a descendant
branch with in-flight, unrelated WIP.

| # | Ticket | Blocked by | Status |
|---|--------|-----------|--------|
| [01](01-config-validation-json-ietf-only.md) | Reject `cisco-ios-xr` + `PROTO`/`JSON` at config-load time | None | done |
| [02](02-materialize-dispatch-and-docs.md) | Narrow `materialize.BuildPlan` dispatch to `JSON_IETF`; fix doc comments | None | done |
| [03](03-adr-live-lab-verification.md) | New ADR: live-lab verification + encoding-restriction decision | 01, 02 | done |
| [04](04-issue-pr-reply.md) | Post follow-up comment on issue #483 / PR #442 with results | 01, 02, 03 | ready-for-agent |

## Working the frontier

1. Scan the table above (re-reading each ticket's own `Status:` line, since that's authoritative, not this table) for tickets that are `ready-for-agent` **and** unblocked (every ticket listed in its "Blocked by" is `Status: done`).
2. Among those, pick the lowest-numbered one — that's the frontier ticket. Right now `01` and `02` are unblocked and independent of each other (can be done in either order or in parallel); `03` and `04` wait on both.
3. On completion, set that ticket's `Status:` line to `done`, update its row in the table above to match, and append a one-line pointer under a `## Comments` heading in the ticket file noting what landed (commit/branch if applicable).
4. If no ticket is both `ready-for-agent` and unblocked, report that the frontier is empty instead of guessing.
