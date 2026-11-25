// Package api holds the domain types, error sentinels and helpers shared by the
// data-server core and its consumers. It is types-only: it imports neither gRPC
// nor any data-server internals. The only sdcpb types it uses are TypedValue,
// Path and Update, the value, address and path-value types of the domain; no
// other sdcpb type may appear here.
package api
