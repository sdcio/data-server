# Path-marker union at northbound render time (Phase 2)

## Parent

[PRD: Sensitive Data Support](../PRD.md)

## What to build

Wire the per-intent `sensitive_paths` slices stored in `sdcio/cache` into the northbound render pipeline. At render time, all intents' slices are loaded and unioned into `RenderOpts.SensitivePathSet`, which the render ops (added in issue 02) already check.

**Union logic**

A helper (e.g. `ops.LoadSensitivePathSet(intents []CacheIntent) map[string]bool`) iterates all loaded intent records, reads each `sensitive_paths` slice, and populates a `map[string]bool`. Key format is keyless XPath (`/bgp/neighbors/auth-password`), consistent with what was stored in issue 03 and what `ops.KeyPrunedPath(e)` produces at render time.

**`GetIntent` path (`pkg/datastore/intent_rpc.go`)**

After loading the intent(s) from cache, call the union helper over all intents for the datastore, populate `RenderOpts.SensitivePathSet`, and pass the opts into `IntentResponseAdapter` / the ops render call. The existing `IncludeSensitive` field on `RenderOpts` is also set from the request flag (wired in issue 02).

**`BlameConfig` path (`pkg/datastore/datastore_rpc.go`)**

The blame processor already loads all intents to compute the merged view. Before invoking `processor_blame_config`, build the union set from the loaded intents and pass it into the processor's render context. The blame processor already checks `SensitivePathSet` (wired in issue 02).

**Cross-intent rule**

If *any* intent marks a path sensitive, all northbound views redact that path — even if the highest-precedence value comes from an intent that did not mark it sensitive. This is enforced naturally by the union: any intent contributing `true` for a path key keeps it in the set.

**Running intent**

The running intent (device-reported state, loaded from the `running` cache entry) is subject to the same obfuscation rule. Include the running entry when building the union if it carries a `sensitive_paths` slice; in practice the running entry will not declare sensitive paths, but the render-time check against the unioned set already covers it.

## Acceptance criteria

- [ ] `GetIntent`: if any intent for the datastore has `/foo/bar` in its `sensitive_paths`, `/foo/bar` is redacted in the `GetIntent` response regardless of which intent holds the winning value
- [ ] `BlameConfig`: same cross-intent rule applies; `Value` and `DeviationValue` for a path in any intent's slice are `***`
- [ ] `GetIntent` with `include_sensitive = true` returns plaintext even when path is in a stored slice
- [ ] `BlameConfig` with `include_sensitive = true` same
- [ ] Intent with no `sensitive_paths` slice (existing intents) contributes nothing to the union — behaviour unchanged
- [ ] Running intent value for a path marked sensitive by any other intent is also redacted
- [ ] Unit test: two intents, only one marks a path sensitive; verify the path is redacted in both `GetIntent` and `BlameConfig`
- [ ] Integration test: replace intent that omits a previously-sensitive path; verify the path is no longer redacted after replace

## Blocked by

Issue 02 (schema-level redaction) — `RenderOpts.SensitivePathSet` and `ops.KeyPrunedPath` must exist.
Issue 03 (path-marker persistence) — `sensitive_paths` must be stored and retrievable from cache.
