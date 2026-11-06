// Package api holds the domain types, error sentinels and helpers shared by the
// data-server core and its consumers. It is types-only: it imports neither gRPC
// nor any data-server internals, so callers can depend on it without pulling in
// the wire layer.
package api
