package api

import (
	"fmt"
	"strings"
)

// RootAmbiguityLookup reports schema-marked ambiguous local names at the YANG root.
type RootAmbiguityLookup interface {
	ModulesForRootLocal(localName string) []string
}

type rootAmbiguityParent interface {
	IsRoot() bool
}

// ValidateBareIdentityAtAmbiguousRoot rejects a bare local name when the schema
// registry marks that name as ambiguous among multiple top-level modules.
// Tree entry creation relies on schema-server GetSchema errors instead; this helper
// is for ingress lint, completions, and other registry-driven UX.
func ValidateBareIdentityAtAmbiguousRoot(parent rootAmbiguityParent, id NodeIdentity, reg RootAmbiguityLookup) error {
	if reg == nil || parent == nil || !parent.IsRoot() || id.Module != "" || id.Local == "" {
		return nil
	}
	mods := reg.ModulesForRootLocal(id.Local)
	if len(mods) == 0 {
		return nil
	}
	return fmt.Errorf(
		"ambiguous node %q at schema root requires module-qualified identity (allowed modules: %s)",
		id.Local,
		strings.Join(mods, ", "),
	)
}
