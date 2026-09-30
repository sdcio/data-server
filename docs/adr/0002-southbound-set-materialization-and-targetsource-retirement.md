# Southbound Set materialization is separated from transport; `TargetSource` is retired from `Target.Set`

Encoding (deciding *what* to send, shaped for the target's NOS) and transport (actually sending it over the wire) are different concerns and must be owned by different layers. `Target.Set` is transport-only: it receives a pre-built `SouthboundSetPlan` and executes it. A dedicated materialization step upstream of `Target.Set` builds the plan from `api.Entry` and `device-profile`.

## Why

`TargetSource` was a serialization interface that forced the driver to re-derive encoding decisions (JSON structure, module grouping, origin fields) from a pre-cooked representation. This embedded NOS-specific encoding knowledge inside the transport layer, where neither `api.Entry` nor `device-profile` was cleanly accessible without re-wrapping.

The raw `api.Entry` is available at every `applyIntent` call site (callers wrap it in an adapter before passing it down). Moving materialization to that boundary — where `d.config.SBI.DeviceProfile` and `api.Entry` are both in scope — means encoding logic is exercised and testable in isolation from any gRPC or device connection.

## Boundary

```
applyIntent(api.Entry, replace bool)
  └─ materialize.BuildPlan(d.config.SBI, api.Entry, replace)
      │  owns: device-profile dispatch, module grouping, origin annotation,
      │         JSON vs proto encoding, merge vs replace semantics
      └─ SouthboundSetPlan{Updates, Replaces, Deletes}
           └─ Target.Set(ctx, plan)
               owns: wire format conversion, gRPC/NETCONF send, response mapping
```

## Considered options

**Rejected — extend `TargetSource` with `GetEntry() api.Entry`:** The driver would have done the NOS-specific encoding internally. NOS knowledge would be scattered across driver implementations, harder to test without a live transport, and would push `api.Entry` into the driver abstraction boundary.

**Accepted — upstream materialization with typed plan:** Encoding in one place (`materialize` package), transport in another (`Target.Set`). The plan type is the contract between them. `TargetSource` is retired from `Set`; Get and sync paths are unchanged in this scope.

## Addendum: the same dispatch discipline applies to Get

The Set-path decision above generalizes: device-profile knowledge belongs behind a seam owned by the target implementation, not inlined as `if cfg.DeviceProfile == X` in code every profile shares. `gnmi.NewTarget` is the analogous sanctioned dispatch point for the Get path — it selects a per-profile request-shaping adapter once (e.g. `sonic.ShapeGetRequest`, defaulting to a no-op) from `cfg.DeviceProfile`, the same way `target.New` selects a `Target` implementation from `cfg.Type`. `gnmiTarget.Get` itself never inspects `DeviceProfile`.
