# External dependencies: goyang fix + sdc-protos fields

## Parent

[PRD: Sensitive Data Support](../PRD.md)

## What to build

Three external PRs that unblock all in-repo implementation work. These can be raised and merged in parallel before any data-server changes land.

**1. `sdcio/goyang` — fix `ApplyDeviate` extension propagation**

`ApplyDeviate` silently drops extensions on `deviate add` / `deviate replace` nodes. A one-line fix copies the deviated node's extensions onto the target:

```go
deviatedNode.Exts = append(deviatedNode.Exts, devSpec.Exts()...)
```

This makes `sdcio-ext:sensitive;` annotations in per-device-profile overlay YANG files survive parsing and appear on the parsed `yang.Entry`.

**2. `sdc-protos` — `LeafSchema` and `LeafListSchema` sensitive flag**

Add `bool sensitive = 22` to `LeafSchema` and `bool sensitive = 24` to `LeafListSchema`. Schema-server sets this flag when it detects the `sdcio-ext:sensitive` extension on a parsed leaf. data-server reads it at northbound render time via `ops.IsSensitive(e)`.

**3. `sdc-protos` — admin bypass on northbound request messages**

Add `bool include_sensitive = <N>` to `GetIntentRequest` and `BlameConfigRequest`. When `true` the render pipeline skips redaction and returns plaintext values. Trust model for v1: any caller that can reach the gRPC port (unchanged from today).

**4. `sdcio/cache` — first-class `sensitive_paths` field on intent record**

Add `repeated string sensitive_paths` to the `tree_persist.Intent` proto (or equivalent intent record type in the cache library). data-server will write this field atomically alongside the intent blob during `TransactionSet` and read it during northbound render. Field absence (existing intents) is treated as non-sensitive — no migration required.

## Acceptance criteria

- [ ] `sdcio/goyang`: `deviate add` with `sdcio-ext:sensitive;` survives `ApplyDeviate` and appears on the parsed `yang.Entry.Exts`
- [ ] `sdc-protos`: `LeafSchema.sensitive` and `LeafListSchema.sensitive` fields present and generated in Go bindings
- [ ] `sdc-protos`: `GetIntentRequest.include_sensitive` and `BlameConfigRequest.include_sensitive` fields present and generated in Go bindings
- [ ] `sdcio/cache`: intent record exposes `sensitive_paths []string`; absent field treated as empty slice by existing readers
- [ ] All four repos have passing CI on their respective PRs
- [ ] data-server `go.mod` updated to reference the new versions of all changed modules

## Blocked by

None — can start immediately.
