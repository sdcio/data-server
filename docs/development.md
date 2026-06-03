# Development

This document holds **contributor conventions** and **design rules** for the data-server repository. Keep it actionable; defer **high-level** system narrative and architectural stance to [architecture.md](./architecture.md) (see its charter: no duplicate of protos, flags, or internal algorithms).

## Scope

- Prefer small, reviewable changes tied to a clear behavior or doc fix.
- Match existing patterns in the nearest package (naming, errors, tests, comments).
- Avoid drive-by refactors in the same change as unrelated feature work unless required for correctness.

## Documentation

- Update [architecture.md](./architecture.md) when you change **externally visible architecture**, **deployment assumptions**, or **documented architectural decisions** — not for every internal refactor; that file’s charter excludes implementation trivia.
- Update [CONTEXT.md](./CONTEXT.md) when you introduce or rename **domain** terms used across SDC (not every internal symbol).
- When changing **`pkg/tree`** (merge rules, processors, importers, validation integration), read [modules/tree.md](./modules/tree.md) for package layout and concepts before deep-diving into code.
- When changing **`pkg/pool`** or parallel tree execution, read [modules/pool.md](./modules/pool.md) so shutdown and error semantics stay consistent.
- Add an ADR under `docs/adr/` only when the bar in [README](./README.md) is met.

## Release tooling

- **`goreleaser check`**: use **GoReleaser v2.14+** (match CI’s `~> v2`). Older CLIs may reject `.goreleaser*.yml` fields such as **`dockers_v2.sbom`** (required so Docker builds do not pass `--attest=type=sbom` on the GitHub Actions default docker driver).

## Testing

- Add or extend tests when behavior changes in a way that is easy to regress; skip tests that only assert trivial wiring unless requested.
- Follow the repository’s existing test layout and helpers.

## Pull requests

- Describe the user-visible or operator-visible effect when there is one.
- Link related issues or design notes when applicable.

*(Expand this file with project-specific rules—for example lint commands, required checks, and import conventions—as the team agrees on them.)*
