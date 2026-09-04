---
Status: accepted
---

# Normalize driver library errors to io.EOF/net.Error at the driver adapter boundary, not in nc.go

`nc.go`'s `isTransportError` classifies transport-level connection drops using only the standard `io`/`net` error vocabulary (`io.EOF`, `net.Error`), so it stays agnostic of which concrete `Driver` implementation is in use. scrapligo (the only current `Driver` implementation, in `driver/scrapligo/`) does not preserve that vocabulary: a real mid-flight transport `EOF` gets swallowed by scrapligo's own channel read loop and re-surfaces as scrapligo's opaque `util.ErrConnectionError` sentinel.

We considered checking `errors.Is(err, scrapligoutil.ErrConnectionError)` directly inside `nc.go`, but decided instead to normalize it to `io.EOF` inside `driver/scrapligo/scrapligo.go` (see `normalizeTransportError`), at the point the error crosses the `Driver` interface. This keeps scrapligo-specific error knowledge contained to its own adapter, consistent with the `Driver` interface abstraction: `nc.go` never needs to import or know about a specific driver library's error types, even as more `Driver` implementations are added.
