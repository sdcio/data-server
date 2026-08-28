# 03 — Sonic `GnmiSetPlan` encoder package

**What to build:** A new, schema-driven encoder package (sibling to `permodule`, exact package name deferred to implementation) with an `Encode(ctx, scb, entry, replace)` entry point matching `permodule.Encode`'s signature shape. Given a changed-entry tree, it produces a `GnmiSetPlan` whose Updates are correctly parent-bound and RFC-7951-unprefixed for SONiC translib to accept, per the rules below. This is the core piece that makes Set operations against a SONiC device actually succeed — it has no knowledge of config or dispatch and can be fully verified with unit tests against fixture trees, independent of the rest of the feature.

**Blocked by:** None — can start immediately. It does not depend on the config-field ticket (01); it takes the same entry/replace-flag shape `permodule.Encode` already takes.

**Status:** done

- [x] For each changed leaf, the encoder finds the leaf's immediate parent and classifies the parent's schema (keyed list vs. plain container) using the existing schema type-switch idiom plus the existing schema-keys helper.
- [x] Plain (non-list) container: all directly-changed sibling leaves under that container are batched into ONE Update at the container's path, with a JSON value containing only those changed leaves.
- [x] Keyed list: all changed leaves under the same list-instance row are batched into ONE Update at the list-instance's path (with its key), whose JSON value is the array-wrapped **full current row** (not just the changed leaves) — this shape is used uniformly for both new-row creation and existing-row field changes.
- [x] Nested containers/lists recurse independently: grouping happens by immediate parent at whatever depth a change occurred, not as one blob per module.
- [x] Every produced Update has `Path.Origin` forced to `"sonic_yang"` regardless of the source entry's origin.
- [x] Each Update's JSON value has RFC 7951 module prefixes (`module:name` → `name`) stripped recursively at every depth in the body (not just the top-level key); path elements are left untouched (no prefix stripping needed there).
- [x] Deletes pass through unmodified as path-only deletes (`GnmiSetPlan.Deletes`), with no JSON shaping.
- [x] The encoder only ever populates `GnmiSetPlan.Updates` (never a Replace-equivalent) — REPLACE semantics are not implemented for this profile.
- [x] When there are no new/changed leaves, the encoder emits nothing (mirrors the existing nil-guard behavior already present for the Cisco encoder).
- [x] Unit tests, table-driven, same style as `permodule`'s encoder tests, covering at minimum: single leaf under a plain container; multiple sibling leaves batched under one plain container; new row created in a keyed list (full-row wrap); existing row's field changed in a keyed list (full-row wrap, same shape); nested module→container→list structure (independent recursion, not one per-module blob); RFC 7951 prefixes at multiple depths including a simulated cross-module augmentation (recursive stripping verified); delete path passthrough with no payload; no-emit behavior when nothing changed.

## Comments

- Landed as `pkg/datastore/target/gnmi/parentbound` on branch `sonic-device-profile` (package name reflects parent-bound encoding shape, sibling to `permodule`).
