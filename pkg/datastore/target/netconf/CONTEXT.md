# NETCONF target

Implements the `Target`/`Driver` interfaces (see `pkg/datastore/target/`) for southbound NETCONF devices, translating `sdcpb` get/set requests into NETCONF RPCs (`get-config`, `edit-config`, `commit`, `discard-changes`) and back.

## Language

**Transport error**:
A failure of the underlying connection to the device *during* an in-flight RPC (e.g. the device restarts mid-`edit-config`) — detected by `isTransportError` — as opposed to a semantic/validation error the device itself returned (e.g. a rejected `edit-config`). Transport errors retire the active session immediately, start a background `reconnect`, and wrap the error in `ErrNotConnected`.

**RPC error**:
An `<rpc-error>` in a reply whose `error-severity` is anything but `warning`, in any namespace prefix. Every `Driver` method fails on one, the same way; warning-only replies succeed and `Set` returns their warnings (from `edit-config` and `commit`) in `SetDataResponse.Warnings`. In candidate mode an RPC error from `edit-config` or `commit` is followed by `discard-changes`, so the rejected change can't be pushed by the next commit.

**Capabilities-changed error** (`ErrCapabilitiesChanged`, defined in `pkg/datastore/target/netconf/types`):
A device-returned `<rpc-error>` carrying `error-severity=error`, `error-tag=resource-denied`, and `error-app-tag=capabilities-changed`, raised when out-of-band changes alter the device's advertised capability set after the initial `<hello>` exchange and the existing NETCONF session can no longer serve RPCs. Unlike validation RPC errors, `ncTarget` retires the stale session immediately, wraps the failure in `ErrNotConnected`, and schedules a fresh session with a cooldown (`defaultCapabilitiesReconnectCooldown`) between capabilities-triggered dials so a misbehaving peer cannot induce a tight reconnect loop.

**ErrNotConnected** (defined in `pkg/datastore/target/types`, not this package):
Within this context, `ErrNotConnected` is deliberately reused across three scenarios under one sentinel: a **pre-flight** check (the target is not connected when a request starts, via `Status().Err()`), a **mid-flight** transport error (the connection dropped during an in-progress RPC, via `isTransportError`), and a **capabilities-changed** session retirement (`ErrCapabilitiesChanged`). All three are wrapped the same way so `translateInternalToGrpcError` maps them to `codes.Unavailable` and upstream retry logic treats them identically — this is an intentional unification, not an accidental conflation of distinct states.
