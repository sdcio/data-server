# Tree

In-process config tree: schema-backed entries, Intent overlays, northbound
render, and sensitivity redaction. Persistence of Intents belongs to Cache;
Tree consumes what datastore loads and decides what plaintext may leave
northbound.

## Language

### Sensitivity

**Schema sensitivity**:
A leaf or leaf-list marked sensitive in the device schema (YANG
`sdcio-ext:sensitive` via schema-server). Honored on every northbound render
regardless of Intent. See ADR 0003.
_Avoid_: "encrypted" (unrelated proto field / future KMS).

**Path marker**:
An Intent-declared schema path that must be redacted on northbound output
even when the schema does not mark the leaf sensitive. Stored per Intent;
contributes to the Live Sensitive Path Index.
_Avoid_: "sensitive path set" as a second concept name when you mean the
live index or a checker; "own markers" as northbound scope policy (retired —
northbound always uses the live union).

**Live Sensitive Path Index**:
The datastore-wide, incrementally maintained union of all Intents' path
markers. The sole path-marker source for northbound redaction (GetIntent,
including Running, BlameConfig, WatchDeviations).
_Avoid_: rebuilding the union at read time; mixing snapshot/`Add` presence
mode into this index.

**Snapshot Sensitive Paths** (`SensitivePaths`):
An immutable `SensitivePathChecker` built from a fixed path slice. Used for
tests and non-datastore helpers — never as the northbound path-marker source.
_Avoid_: `Add` on the live index; calling this "the index."

**Sensitive Render** (`SensitiveRender`):
The single tree-owned northbound redaction context: expose/include-sensitive
flag plus a path-marker checker, resolving a leaf to redacted-or-real before
formatters run. Embedded in `RenderOpts` so callers use constructors
(`RenderOptsNorthbound`, `RenderOptsRevealAll`) instead of setting include /
path-marker fields.
_Avoid_: each formatter re-implementing `if sensitive → "***"`; datastore
RPCs hand-assembling include / path-marker fields on `RenderOpts`.

**Redaction sentinel**:
The substituted northbound value for a redacted leaf (`***` /
`RedactedTypedValue`). Formatters must not invent their own placeholder.
