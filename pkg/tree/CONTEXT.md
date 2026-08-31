# Configuration tree

The YANG-aware in-memory configuration graph: nodes, leaves, owners, and merge semantics used for apply, export, validation, and deviation detection.

## Language

**Precedence**:
Which owner's value wins when multiple intents (or running) set the same leaf. Lower numeric priority wins; when priorities are equal, lower timestamp wins; when timestamps also tie, ascending owner-name order wins.
_Avoid_: priority alone — priority is only the first comparison step.

**Precedence tiebreak**:
The ordered fallback when two owners share the same priority on a leaf: timestamp (earlier wins), then owner name (lexicographically smaller wins). Applies at leaf level and in YANG choice-case resolution.
_Avoid_: insertion order, first-seen — those describe the pre-tiebreak behaviour and are not stable across reloads.

**Owner**:
The intent name (or `running` / `default`) that supplied a leaf value. Each leaf can hold one entry per owner; merge picks the highest precedence among them.
_Avoid_: source, writer — too generic and collide with southbound/northbound vocabulary elsewhere.

**Device-apply view**:
Serialization of an `api.Entry` for Set to the device; never redacts sensitive values.
_Avoid_: southbound output, EntryOutputAdapter

**Intent response view**:
Serialization of an intent (or running) tree for GetIntent; redaction governed by `RenderOpts`.
_Avoid_: northbound export, client response adapter

**Key level**:
A tree depth still consuming one of a list's key components — the node carries no resolved instance yet. A list with N keys has N key levels between its schema-bearing node and its instances.
_Avoid_: list-definition node — not existing codebase vocabulary.

**Instance**:
A fully-keyed list entry — the tree node reached after descending through all of a list's key levels, one per key. `must`/mandatory checks that reference sibling fields are only meaningful once at instance level.
