package proto

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/proto"
)

// FilterIntentForScopes returns a copy of intent with config trimmed to branches
// that overlap any revert scope path. Explicit deletes and metadata are preserved.
func FilterIntentForScopes(intent *tree_persist.Intent, scopes []*sdcpb.Path) *tree_persist.Intent {
	if intent == nil || len(scopes) == 0 {
		return intent
	}
	out := &tree_persist.Intent{
		IntentName:   intent.GetIntentName(),
		Priority:     intent.GetPriority(),
		NonRevertive: intent.GetNonRevertive(),
	}
	if len(intent.GetExplicitDeletes()) > 0 {
		out.ExplicitDeletes = filterExplicitDeletes(intent.GetExplicitDeletes(), scopes)
	}
	if intent.GetRoot() != nil {
		out.Root = filterTreeElement(intent.GetRoot(), scopes, &sdcpb.Path{})
	}
	return out
}

func filterExplicitDeletes(deletes []*sdcpb.Path, scopes []*sdcpb.Path) []*sdcpb.Path {
	if len(deletes) == 0 {
		return nil
	}
	var out []*sdcpb.Path
	for _, del := range deletes {
		if del == nil {
			continue
		}
		if len(del.GetElem()) == 0 {
			out = append(out, del)
			continue
		}
		for _, scope := range scopes {
			if scope != nil && (del.IsParentPathOf(scope) || scope.IsParentPathOf(del) || del.PathsEqual(scope)) {
				out = append(out, del)
				break
			}
		}
	}
	return out
}

func filterTreeElement(el *tree_persist.TreeElement, scopes []*sdcpb.Path, prefix *sdcpb.Path) *tree_persist.TreeElement {
	if el == nil {
		return nil
	}
	filtered := &tree_persist.TreeElement{
		Name:        el.GetName(),
		LeafVariant: el.GetLeafVariant(),
	}
	for _, ch := range el.GetChilds() {
		if !scopeSelectsChildAt(prefix, scopes, ch) {
			continue
		}
		childPrefix := prefix.CopyPathAddElem(sdcpb.NewPathElem(ch.GetName(), nil))
		if fc := filterTreeElement(ch, scopes, childPrefix); fc != nil {
			filtered.Childs = append(filtered.Childs, fc)
		}
	}
	if len(filtered.GetLeafVariant()) > 0 || len(filtered.GetChilds()) > 0 || len(prefix.GetElem()) == 0 {
		return filtered
	}
	return nil
}

func subtreeOverlapsScopes(prefix *sdcpb.Path, scopes []*sdcpb.Path) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		if prefix.SharesPrefix(scope) || scope.SharesPrefix(prefix) || pathPrefixMatches(prefix, scope) || pathPrefixMatches(scope, prefix) {
			return true
		}
	}
	return false
}

func scopeSelectsChildAt(prefix *sdcpb.Path, scopes []*sdcpb.Path, child *tree_persist.TreeElement) bool {
	childName := child.GetName()
	if len(scopes) == 0 {
		return true
	}
	depth := len(prefix.GetElem())
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		elems := scope.GetElem()
		if len(elems) <= depth {
			if prefix.SharesPrefix(scope) || scope.SharesPrefix(prefix) {
				return true
			}
			continue
		}
		next := elems[depth]
		if next.GetName() != childName {
			continue
		}
		if keysMatch(child, next.GetKey()) {
			return true
		}
	}
	return false
}

func pathPrefixMatches(prefix, path *sdcpb.Path) bool {
	pa, pb := prefix.GetElem(), path.GetElem()
	if len(pa) > len(pb) {
		return false
	}
	for i := range pa {
		if pa[i].GetName() != pb[i].GetName() {
			return false
		}
	}
	return true
}

// keysMatch reports whether a list entry element carries the given key values.
// Persisted list entries repeat the list name and hold keys as leaf children.
func keysMatch(el *tree_persist.TreeElement, keys map[string]string) bool {
	for name, want := range keys {
		found := false
		for _, c := range el.GetChilds() {
			if c.GetName() != name {
				continue
			}
			tv := &sdcpb.TypedValue{}
			if err := proto.Unmarshal(c.GetLeafVariant(), tv); err != nil || tv.ToString() != want {
				return false
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}
