# 01 — Standalone namespace/name split helper

**What to build:** A standalone, pure helper (in the `cache` package, alongside `configserver.go`) that splits a datastore name (`cacheInstanceName`, e.g. `"prod.srl1"`) into a `configserver.Target{Namespace, Name}` by splitting on the *first* `.` only. It returns a new package-level sentinel error, `ErrMalformedDatastoreName` (matching the existing `ErrNotFound`/`ErrRunningNotFound` convention), whenever there's no dot at all, or when either resulting segment (namespace or name) is empty — e.g. `".srl1"` or `"prod."`. The helper's contract is "return a `Target` with both fields guaranteed non-empty, or fail," not merely "check a dot exists." No caller wiring yet — this ticket only adds the helper and its direct unit tests.

**Blocked by:** None — can start immediately

**Status:** done

- [x] New pure function exists, taking a datastore name string and returning `(configserver.Target, error)`
- [x] Splits on the first `.` only (a name like `"prod.rack1.srl1"` yields namespace `"prod"`, name `"rack1.srl1"`)
- [x] Returns `ErrMalformedDatastoreName` for: no dot at all, empty namespace segment (`".srl1"`), empty name segment (`"prod."`)
- [x] `ErrMalformedDatastoreName` is a package-level sentinel, checkable via `errors.Is`
- [x] Table-driven unit test covers: dotted (`"prod.srl1"`), dotted-with-extra-dots-in-name (`"prod.rack1.srl1"`), no-dot (error), empty-namespace (error), empty-name (error)
- [x] `go build ./...`, `go vet ./...`, `go test ./pkg/cache/...` pass

## Comments

- Landed in `pkg/cache/configserver_split.go` (+ `configserver_split_test.go`): `splitDatastoreName` helper and `ErrMalformedDatastoreName` sentinel, table-driven test per the Testing Decisions cases.
