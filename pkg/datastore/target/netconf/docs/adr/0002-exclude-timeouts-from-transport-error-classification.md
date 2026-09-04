---
Status: accepted
---

# Exclude scrapligo's timeout sentinel from transport-error classification

`isTransportError`/`normalizeTransportError` treat a connection drop (`io.EOF`, `net.Error`, or scrapligo's `util.ErrConnectionError`) as a mid-flight transport error that triggers `t.Close`+`reconnect` and gets wrapped in `targettypes.ErrNotConnected`, which `translateInternalToGrpcError` maps to `codes.Unavailable` so config-server retries the transaction.

We deliberately did **not** extend this to scrapligo's `util.ErrTimeoutError` (channel/write/read op timeouts), even though a hung operation against a dead socket can look similar to a dropped one. A timeout can just as easily mean a slow-but-alive device (e.g. a large `commit` under load), and misclassifying that as "not connected" would make config-server retry transactions against devices that are still processing the original one — a different, worse failure mode than the one this fix addresses. If a real-world case shows scrapligo timeouts reliably correlating with dead connections, revisit this by distinguishing "timeout because socket is dead" from "timeout because device is slow" rather than lumping all timeouts into `ErrNotConnected`.
