# Intent vs Running Client return-type split

**Status:** accepted

**Pairs with:** `.scratch/open/03-split-intent-metadata-off-import-adapter.md`

`ImportConfigAdapter` grew `GetOrphan` / `GetSensitivePaths` so cache backends could surface Intent metadata through the same type used for tree walk. JSON/XML forever stubbed those accessors; the import processor never reads them. Every new Intent-only field would force another stub on device/running importers — classic shallow widening.

**Decision:** narrow `ImportConfigAdapter` to tree walk plus import-needed metadata (name, priority, deletes, non-revertive). Introduce `IntentAdapter` (`ImportConfigAdapter` + `GetOrphan` / `GetSensitivePaths`) in `pkg/tree/importer`. Client Intent reads (`InstanceIntentGet` / `InstanceIntentGetAll`, and their bound equivalents) return `IntentAdapter`; Running reads (`InstanceRunningGet`) stay `ImportConfigAdapter`. `documentImporter` and `ProtoTreeImporter` implement `IntentAdapter`; JSON/XML do not.

## Considered options

- **Type-assert Orphan/SensitivePaths at Intent call sites.** Rejected: silent miss when a non-Intent adapter arrives; callers must remember to assert.
- **Separate Client methods for Intent metadata.** Rejected: extra hop for every Intent Get; metadata belongs with the adapter already in hand.
- **Keep a single return type; leave stubs on JSON/XML.** Rejected: fails the deletion test — every new Intent field forces another stub.
- **Name `IntentDescriptor`.** Rejected: clashes with northbound response vocabulary; `IntentAdapter` mirrors `ImportConfigAdapter` and stays distinct from `IntentResponseAdapter`.
- **House `IntentAdapter` under `pkg/cache`.** Rejected: would create a cache←tree reverse dependency or force a wrapper; it lives beside `ImportConfigAdapter` in `pkg/tree/importer`.

## Consequences

Intent-only metadata no longer pollutes device/running importers. `populateSensitivePathIndex` and GetIntent read Orphan/SensitivePaths through `IntentAdapter`. `configsnapshot.NewImportAdapter` returns `IntentAdapter`. Expanding GetIntentResponse / TransactionIntent / config-server wire is out of scope for this split.
