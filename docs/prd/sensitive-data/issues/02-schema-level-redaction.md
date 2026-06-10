# Schema-level render redaction + admin bypass (Phase 1)

## Parent

[PRD: Sensitive Data Support](../PRD.md)

## What to build

Implement the schema-level (static baseline) redaction layer end-to-end: from schema flag detection through all northbound render paths, covering `GetIntent`, `BlameConfig`, and the admin bypass.

---

### `ops.IsSensitive(e api.Entry) bool`

New helper in `pkg/tree/ops`. Reads `LeafSchema.sensitive` / `LeafListSchema.sensitive` from the entry's schema via a type-switch on `e.GetSchema().GetSchema().(type)`. Returns `false` when schema is absent (non-leaf nodes, existing intents without schema flags).

---

### `ops.KeyPrunedPath(e api.Entry) string`

Returns `e.SdcpbPath().ToXPath(true)` — the existing `noKeys=true` argument already strips list key predicates. Format: `/bgp/neighbors/auth-password`. This is the canonical sensitive-path lookup key used both at render time and in stored `sensitive_paths` markers (Issue 03).

---

### `RenderOpts` — struct hierarchy in `pkg/tree/ops/render_opts.go`

Replace the current per-function boolean parameters with structured option types. The existing `onlyNewOrUpdated` bool is absorbed into `RenderOpts.OnlyNewOrUpdated`.

```go
// ops/render_opts.go

// RenderOpts are the common render options shared across all northbound
// render operations.
type RenderOpts struct {
    OnlyNewOrUpdated bool
    IncludeSensitive  bool
    // SensitivePathSet is the union of sensitive_paths slices read from all
    // in-scope intents (populated in Issue 04). An empty/nil map means no
    // path-marker-based redaction is applied (correct for schema-only Phase 1).
    SensitivePathSet map[string]bool
}

type XMLRenderOpts struct {
    RenderOpts
    HonorNamespace         bool
    OperationWithNamespace bool
    UseOperationRemove     bool
}

type XPathRenderOpts struct {
    RenderOpts
    IncludeDefaults bool
}
```

New signatures:

```go
ToJson(ctx, entry, RenderOpts) (any, error)
ToJsonIETF(ctx, entry, RenderOpts) (any, error)
ToProtoUpdates(ctx, entry, RenderOpts) ([]*sdcpb.Update, error)
ToXML(ctx, entry, XMLRenderOpts) (*etree.Document, error)
ToXPath(ctx, entry, XPathRenderOpts) (*sdcpb.PathValues, error)
```

Call sites to update (all currently in the same package or one hop away):
- `pkg/tree/api/adapter/intentresponse.go` — `IntentResponseAdapter.ToJson`, `.ToJsonIETF`, `.ToXML`, `.ToProtoUpdates`, `.ToXPath` (all currently hardcode `false` for `onlyNewOrUpdated`)
- `pkg/tree/ops/xml_test.go`, `json_test.go`, `xpath_test.go` — test structs use named bool params, migrate to `RenderOpts{...}` / `XMLRenderOpts{...}` / `XPathRenderOpts{...}`
- `pkg/datastore/intent_rpc.go` — calls ops indirectly via the adapter; will thread `IncludeSensitive` here in step 7

---

### Redaction condition

At the leaf/leaf-list emit point in each render op:

```go
!opts.IncludeSensitive && (ops.IsSensitive(e) || opts.SensitivePathSet[ops.KeyPrunedPath(e)])
```

Redacted values are emitted as:
- JSON/XML: string `"***"`
- Proto (`TypedValue`): `&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "***"}}`
- XPath (`PathValues`): same `TypedValue` sentinel

The field is **always present** in the response — never omitted.

---

### `processor_blame_config.go`

Add `IncludeSensitive bool` to `BlameConfigProcessorParams`. Apply the same redaction condition at both `t.self.SetValue(...)` and `t.self.SetDeviationValue(...)` assignments in `BlameConfigTask.Run`.

---

### Threading `include_sensitive` from request to render

| Layer | File | Change |
|---|---|---|
| gRPC handler | `pkg/server/intent.go` | Read `req.GetIncludeSensitive()`, pass to `ds.GetIntent(ctx, name, exposeSensitive)` |
| gRPC handler | `pkg/server/datastore.go` | Read `req.GetIncludeSensitive()`, pass to `ds.BlameConfig(ctx, req)` |
| Datastore | `pkg/datastore/intent_rpc.go` | Accept `exposeSensitive bool`, build `RenderOpts{IncludeSensitive: exposeSensitive}`, pass to adapter |
| Datastore | `pkg/datastore/datastore_rpc.go` | Same — pass into `BlameConfigProcessorParams` |

---

## Acceptance criteria

- [ ] `ops.IsSensitive(e)` returns `true` for a leaf whose schema has `sensitive = true`; `false` for all other entries
- [ ] `ops.KeyPrunedPath(e)` returns a keyless XPath string (no `[key=value]` predicates); verified by unit test
- [ ] All of `ToJson`, `ToJsonIETF`, `ToProtoUpdates`, `ToXML`, `ToXPath` emit `***` for a sensitive leaf when `IncludeSensitive = false`
- [ ] Same render ops return the real value when `IncludeSensitive = true`
- [ ] `BlameConfig` response: `Value` and `DeviationValue` for a sensitive leaf are `***` when `include_sensitive` not set
- [ ] `GetIntent` response: sensitive leaf values are `***` when `include_sensitive` not set
- [ ] `GetIntent` and `BlameConfig` with `include_sensitive = true` return plaintext values
- [ ] Southbound path (tree merge, YANG validation, target apply) is unaffected — no schema flag checks added there
- [ ] Existing intents without `sensitive` schema flags behave identically to today
- [ ] Integration test: submit an intent with a schema-marked-sensitive leaf; verify `GetIntent` returns `***`; verify with `include_sensitive = true` returns plaintext

## Blocked by

Issue 01 (external deps) — needs `LeafSchema.sensitive`, `LeafListSchema.sensitive`, and `include_sensitive` proto fields. ✅ Done.
