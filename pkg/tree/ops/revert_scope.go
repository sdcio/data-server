package ops

import (
	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// ScopesCoverTree reports whether one of the scopes is the root, hence the entire tree.
func ScopesCoverTree(scopes *sdcpb.PathSet) bool {
	if scopes == nil {
		return false
	}
	for s := range scopes.Items() {
		if len(s.GetElem()) == 0 {
			return true
		}
	}
	return false
}

// PathSetIsEmpty reports whether the set is nil or holds no paths.
func PathSetIsEmpty(ps *sdcpb.PathSet) bool {
	if ps == nil {
		return true
	}
	for range ps.Items() {
		return false
	}
	return true
}

// RevertScope returns the path of the subtree that has to be present to evaluate precedence and
// choice/case for a touched entry (see "Touched entries"): the entry's parent, or the entry itself
// if it is a top level entry.
//
// If the scope is an element of a case, the other cases of the choice are its siblings and compete with it.
// The scope is therefore lifted to the entry that owns the choice, repeatedly for nested choices.
func RevertScope(e api.Entry) *sdcpb.Path {
	s := e
	// a top level entry has no parent scope below the root, keep the entry itself.
	if p := e.GetParent(); p != nil && !p.IsRoot() {
		s = p
	}
	s = liftToIncompleteKeys(s)
	// Any entry on the way up can be an element of a case, not only the scope itself: for a scope deep
	// inside a case it is one of its ancestors. The highest choice owner found wins.
	scope := s
	for cur := s; cur != nil && !cur.IsRoot(); cur = cur.GetParent() {
		elem := schemaElementOf(cur)
		if elem == nil || elem.IsRoot() {
			continue
		}
		owner, _ := GetFirstAncestorWithSchema(elem)
		if owner != nil && owner.ChoicesResolvers().HasElement(elem.PathName()) {
			// elem.GetParent() is the owner itself or, for a list entry owning the choice, its last key level.
			scope = elem.GetParent()
		}
	}
	s = scope
	// scopes are plain element paths, not root based
	return &sdcpb.Path{Elem: s.SdcpbPath().GetElem()}
}

// schemaElementOf returns the entry that carries the schema for s. For key level entries
// of a list this is the list entry itself, which is what a choice refers to by name.
func schemaElementOf(s api.Entry) api.Entry {
	if s.GetSchema() != nil {
		return s
	}
	elem, _ := GetFirstAncestorWithSchema(s)
	return elem
}

// liftToIncompleteKeys returns the list entry for a key level entry that does not carry all keys yet.
// The path of such an entry would be a partial key and does not identify a subtree.
func liftToIncompleteKeys(s api.Entry) api.Entry {
	if s.GetSchema() != nil {
		return s
	}
	list, levelsUp := GetFirstAncestorWithSchema(s)
	if list == nil {
		return s
	}
	if c := list.GetSchema().GetContainer(); c != nil && levelsUp < len(c.GetKeys()) {
		return list
	}
	return s
}
