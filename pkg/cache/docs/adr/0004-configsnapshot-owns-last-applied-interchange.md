# Extract last-applied interchange into `pkg/cache/configsnapshot`

**Status:** accepted

**Supersedes in part:** [ADR 0002](0002-segregate-cache-client-by-capability.md)'s Consequences relocate of `documentImporter`/`mergeConfigBlobs` into `pkg/tree/importer/configserver/`, and revisits that ADR's deferred standalone `DocumentTreeImporter` type. Capability segregation and the rest of 0002 stand unmodified.

**Pairs with:** `.scratch/open/01-own-document-round-trip.md` (implementation ticket).

ADR 0003 made last-applied writes real under `Cache.Type: config-server`. The write path flattens `tree_persist.Intent` into the ConfigSnapshotService DTO (`Document` — a Go type name only, not a domain term; see `CONTEXT.md` Last-applied), and the read path merges that DTO back into an `ImportConfigAdapter`. Those two halves of one interchange lived in different packages (`pkg/cache/configserver` vs `pkg/tree/importer/configserver`), with a package cycle on the DTO and no single place to ask “what does a Modify round-trip look like?”

**Decision:** own the interchange in `pkg/cache/configsnapshot` — `Target`, `Document`, `ConfigBlob`, `DocumentFromIntent`, blob merge, and `NewImportAdapter` as package functions. `pkg/cache/configserver` keeps only the remote port (`LocalConfigClient` / gRPC / fake) and imports the DTO from `configsnapshot`. `ConfigServerCache` only orchestrates (codec ↔ client). Delete `pkg/tree/importer/configserver` (no re-export shim). Prefer round-trip tests in `configsnapshot`, plus a Fake Modify→Get that asserts tree content.

## Considered options

- **Keep halves split; add a cross-package round-trip test only.** Rejected: fixes coverage, not locality or the DTO cycle.
- **Expand `pkg/tree/importer/configserver`** (move DTO + flatten in). Rejected: ConfigSnapshotService-shaped types are not tree importers; would still couple the port package oddly.
- **Expand `pkg/cache/configserver`** (move merge/adapter in). Rejected: bloated the remote-port package and reversed 0002's “importers with importers” for the wrong reason — the codec is not the port.
- **Named codec / `DocumentTreeImporter` type.** Rejected for now (same reason 0002 deferred it): one call site, no behavior to vary; package functions suffice. Revisit if a second Document-shaped source appears.
- **Amend ADR 0002 in place.** Rejected: 0002 records a relocate that shipped; a new ADR records why that package home is superseded without falsifying history.

## Consequences

Dependency direction becomes one-way: `configserver` (port) → `configsnapshot` → `tree/importer` (+ json). Ticket 03 (splitting Orphan/SensitivePaths off `ImportConfigAdapter`) should land after this extract so the metadata shrink hits one package.
