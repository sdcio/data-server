# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT-MAP.md`** at the repo root — it points at one `CONTEXT.md` per context. Read each one relevant to the topic.
- **`docs/adr/`** at the repo root — system-wide decisions (this repo already has ADRs here, e.g. `docs/adr/0001-ios-xr-granular-gnmi-json-from-api-entry.md`). Also check `pkg/<context>/docs/adr/` for context-scoped decisions once a context has its own ADR directory.

If `CONTEXT-MAP.md` or a given context's `CONTEXT.md` don't exist yet, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

This repo is multi-context (Go, organized by `pkg/<context>/` rather than `src/<context>/`):

```
/
├── CONTEXT-MAP.md
├── docs/adr/                          ← system-wide decisions (already populated)
└── pkg/
    ├── tree/
    │   ├── CONTEXT.md
    │   └── docs/adr/                  ← context-specific decisions
    ├── datastore/
    │   ├── CONTEXT.md
    │   └── docs/adr/
    └── schema/
        ├── CONTEXT.md
        └── docs/adr/
```

Contexts and their `CONTEXT.md` files don't need to exist upfront — treat the tree above as illustrative, not prescriptive. `/domain-modeling` decides which `pkg/` subtree deserves its own context doc as terms and decisions get resolved for it.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in the relevant `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0002 (southbound set materialization) — but worth reopening because…_
