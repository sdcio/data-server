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
