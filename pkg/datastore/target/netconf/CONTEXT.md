# NETCONF target

Implements the `Target`/`Driver` interfaces (see `pkg/datastore/target/`) for southbound NETCONF devices, translating `sdcpb` get/set requests into NETCONF RPCs (`get-config`, `edit-config`, `commit`, `discard-changes`) and back.

## Language

**Transport error**:
A failure of the underlying connection to the device *during* an in-flight RPC (e.g. the device restarts mid-`edit-config`) — detected by `isTransportError` — as opposed to a semantic/validation error the device itself returned (e.g. a rejected `edit-config`). Only transport errors trigger `Close`+`reconnect` and get wrapped in `ErrNotConnected`.

**ErrNotConnected** (defined in `pkg/datastore/target/types`, not this package):
Within this context, `ErrNotConnected` is deliberately reused for two distinct scenarios under one sentinel: a **pre-flight** check (the target was never connected when a request started, via `Status().Err()`) and a **mid-flight** transport error (the target *was* connected but the transport dropped during an in-progress RPC, via `isTransportError`). Both are wrapped the same way so `translateInternalToGrpcError` maps either to `codes.Unavailable` and config-server's retry logic treats them identically — this is an intentional unification, not an accidental conflation of two different states.
