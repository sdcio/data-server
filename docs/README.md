# Documentation

What this tree keeps — and why.

## Keep

| Path | Purpose |
|------|---------|
| [`docs/adr/`](./adr/) | System-wide architecture decisions (hard to reverse, surprising without context). |
| [`pkg/<context>/docs/adr/`](../pkg/) | Context-scoped decisions (e.g. `pkg/cache/docs/adr/`, `pkg/tree/docs/adr/`). |
| [`pkg/<context>/CONTEXT.md`](../pkg/) | Domain vocabulary per context, created lazily. |
| [`docs/agents/`](./agents/) | How agent skills find issues and domain docs. |
| [`AGENTS.md`](../AGENTS.md) | Entry pointers for agents. |

Active specs and tickets live under [`.scratch/<feature-slug>/`](../.scratch/), not here. See [`agents/issue-tracker.md`](./agents/issue-tracker.md).

## Do not add here

Narrative architecture tours, package maps, finished PRDs, or anything that restates what code, protos, tests, or config already prove. If the durable residue of a change is a *decision*, write an ADR; if it is a *term*, update `CONTEXT.md`.
