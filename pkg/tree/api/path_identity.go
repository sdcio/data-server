package api

import (
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// NodeIdentityFromPathElem derives tree node identity from a path segment.
// For the first element (index 0), path.Origin is applied when the name has no module prefix.
func NodeIdentityFromPathElem(pe *sdcpb.PathElem, path *sdcpb.Path, elemIndex int) NodeIdentity {
	if pe == nil {
		return NodeIdentity{}
	}
	id := ParseJSONIETFKey(pe.GetName())
	if id.Module == "" && elemIndex == 0 && path != nil {
		if origin := path.GetOrigin(); origin != "" {
			id.Local = pe.GetName()
			id.Module = origin
		}
	}
	if id.Local == "" {
		id.Local = pe.GetName()
	}
	return id
}

// LookupChild resolves an active child for a path segment using NodeIdentity.
func LookupChild(childs map[string]Entry, pe *sdcpb.PathElem, path *sdcpb.Path, elemIndex int) (Entry, bool) {
	if pe == nil {
		return nil, false
	}
	id := NodeIdentityFromPathElem(pe, path, elemIndex)
	child, ok := childs[id.MapKey()]
	return child, ok
}

// MatchesPathElemPrefix reports whether a partial path segment matches this identity.
func (id NodeIdentity) MatchesPathElemPrefix(partial *sdcpb.PathElem) bool {
	if partial == nil {
		return true
	}
	partialID := ParseJSONIETFKey(partial.GetName())
	if partialID.Module != "" {
		if partialID.Local != "" && !strings.HasPrefix(id.Local, partialID.Local) {
			return false
		}
		if !strings.HasPrefix(id.Module, partialID.Module) {
			return false
		}
		return strings.HasPrefix(id.JSONIETFKey(), partial.GetName())
	}
	return strings.HasPrefix(id.Local, partial.GetName())
}

// PersistName returns the name used in tree_persist and similar exports (module:local when set).
func (id NodeIdentity) PersistName() string {
	if id.Module == "" {
		return id.Local
	}
	return id.JSONIETFKey()
}

// ApplyModuleToSchemaLookupPath adjusts a schema lookup path for a module-qualified identity.
func ApplyModuleToSchemaLookupPath(path *sdcpb.Path, id NodeIdentity, parentIsRoot bool) {
	if path == nil || id.Module == "" {
		return
	}
	elems := path.GetElem()
	if len(elems) == 0 {
		return
	}
	if parentIsRoot {
		path.Origin = id.Module
		return
	}
	last := elems[len(elems)-1]
	if !strings.Contains(last.GetName(), ":") {
		last.Name = id.JSONIETFKey()
	}
}
