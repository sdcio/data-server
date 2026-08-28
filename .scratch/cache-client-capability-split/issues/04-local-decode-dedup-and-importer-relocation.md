# 04 — `LocalCache` decode dedup + relocate config-server's `ImportConfigAdapter`

**What to build:** Two independent, mechanical cleanups bundled together since both are small and orthogonal to the interface split (Track A). First: `LocalCache` (`pkg/cache/local.go`) gains a private `decodeIntent(b []byte) (importer.ImportConfigAdapter, error)` method — wrapping the existing `proto.Unmarshal` into `tree_persist.Intent` followed by `treeproto.NewProtoTreeImporter` — called from `InstanceIntentGet`, the `InstanceIntentGetAll` channel bridge, and `InstanceRunningGet`, replacing the three duplicated inline decode blocks with zero behavior change. Second: config-server's `ImportConfigAdapter` implementation — `documentImporter` and `mergeConfigBlobs` (and their tests) — moves unchanged from `pkg/cache/configserver/{importer.go,merge.go,importer_test.go,merge_test.go}` to a new `pkg/tree/importer/configserver/` package, alongside its sibling tree importers (`pkg/tree/importer/{proto,json,xml}/`); `ConfigServerCache.InstanceIntentGet`/`InstanceIntentGetAll` (`pkg/cache/configserver.go`) update their imports to the new location. `pkg/cache/configserver/` retains only the reader seam afterward: `localread.go`, `grpc.go`, `fake.go` (+ their tests). No standalone `DocumentTreeImporter` type is introduced — `documentImporter` keeps embedding `JsonTreeImporter` and overriding `GetOrphan`/`GetSensitivePaths`, just relocated as-is.

**Blocked by:** None — can start immediately, independent of tickets 01–03.

**Status:** done

- [x] `pkg/cache/local.go` has a private `decodeIntent(b []byte) (importer.ImportConfigAdapter, error)` method; `InstanceIntentGet`, the `InstanceIntentGetAll` channel bridge, and `InstanceRunningGet` all call it instead of inlining `proto.Unmarshal`/`treeproto.NewProtoTreeImporter`.
- [x] `pkg/cache/local_test.go` confirms `InstanceIntentGet`, `InstanceIntentGetAll`, and `InstanceRunningGet` all still round-trip a `*tree_persist.Intent` correctly after the extraction (regression check, not new behavior).
- [x] `documentImporter`, `mergeConfigBlobs`, and their tests move unchanged to `pkg/tree/importer/configserver/`.
- [x] `pkg/cache/configserver/` contains only `localread.go`, `grpc.go`, `fake.go` (+ their tests) afterward.
- [x] `ConfigServerCache.InstanceIntentGet`/`InstanceIntentGetAll` import `pkg/tree/importer/configserver` instead of the old in-package location.
- [x] No standalone `DocumentTreeImporter` type is introduced (out of scope per spec — deferred).
- [x] `go build ./...`, `go vet ./...`, and `go test ./...` all pass.

## Comments

- Landed: `LocalCache.decodeIntent` extracted (with a new `TestLocalCache_InstanceIntentGetRoundTrip` regression test added first, since single-get round-trip wasn't covered yet); `documentImporter`/`mergeConfigBlobs` + tests relocated to `pkg/tree/importer/configserver` (referencing `pkg/cache/configserver.Document`/`ConfigBlob` via a `csreader` import alias, since both packages share the name `configserver`); `pkg/cache/configserver.go` updated to call the new `csimporter.NewImportAdapter`. `go build`/`vet`/`test ./...` all pass.
