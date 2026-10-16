package api

import "strings"

// NodeIdentity is the canonical identity of a tree node: YANG local name plus
// defining module (RFC 7951 / gNMI Origin). Schema details stay on Entry.GetSchema().
type NodeIdentity struct {
	Local  string
	Module string
}

// LocalIdentity is a node identity with an empty module (unambiguous paths).
func LocalIdentity(local string) NodeIdentity {
	return NodeIdentity{Local: local, Module: ""}
}

// MapKey returns the canonical key for ChildMap and EntryMap storage.
func (id NodeIdentity) MapKey() string {
	if id.Module == "" {
		return id.Local
	}
	return id.Module + ":" + id.Local
}

// JSONIETFKey returns the RFC 7951 JSON key form (module:local or bare local).
func (id NodeIdentity) JSONIETFKey() string {
	return id.MapKey()
}

// GNMIOrigin returns the module name for gNMI Path.Origin when applicable.
func (id NodeIdentity) GNMIOrigin() string {
	return id.Module
}

// IsZero reports whether the identity has no local name.
func (id NodeIdentity) IsZero() bool {
	return id.Local == ""
}

// ParseJSONIETFKey parses a JSON_IETF object key into NodeIdentity.
// Keys without ":" are treated as bare local names.
func ParseJSONIETFKey(key string) NodeIdentity {
	if key == "" {
		return NodeIdentity{}
	}
	module, local, ok := strings.Cut(key, ":")
	if !ok {
		return LocalIdentity(key)
	}
	return NodeIdentity{Local: local, Module: module}
}
