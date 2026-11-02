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

**Live Sensitive Path Index** (`sensitive.PathIndex`):
The datastore-wide, incrementally maintained union of all Intents' path
markers. The sole path-marker source for northbound redaction (GetIntent,
including Running, BlameConfig, WatchDeviations). Consumed through the
`sensitive.PathChecker` interface.
_Avoid_: rebuilding the union at read time; mixing snapshot/`Add` presence
mode into this index; a second immutable checker implementation (the former
`SensitivePaths` was removed — tests use a `PathIndex`).

**Sensitive Render** (`sensitive.Render`):
The single northbound redaction context: include-sensitive flag plus a
path-marker checker, resolving a leaf to redacted-or-real before formatters
run. Lives in `pkg/tree/sensitive`, below both `api` and `ops`, so render,
blame and deviation code all redact through the same type. Embedded in
`ops.RenderOpts`; callers use `ops.RenderOptsNorthbound` /
`ops.RenderOptsRevealAll` (or `sensitive.NewRender` / `sensitive.RevealAll`)
instead of setting include / path-marker fields.
_Avoid_: each formatter re-implementing `if sensitive → "***"`; passing a
pre-computed `shouldRedact bool` down the tree; datastore RPCs hand-assembling
include / path-marker fields on `RenderOpts`.

**Redaction sentinel**:
The substituted northbound value for a redacted leaf (`***` /
`sensitive.RedactedTypedValue`). Formatters must not invent their own placeholder.
