package types

// DescendMethod selects how direct child nodes are filtered when reading an Entry's
// children. It does not control recursion depth; callers walk the tree themselves.
//
// In YANG terms, DescendMethodActiveChilds hides child nodes that belong to choice
// cases that lost precedence (see api.ChoiceResolvers.GetSkipElements).
// DescendMethodAll returns every stored child regardless of choice resolution.
type DescendMethod int

const (
	// All direct children, including branches under non-winning choice cases.
	DescendMethodAll DescendMethod = iota
	// Direct children omitting choice-case elements on the skip list.
	DescendMethodActiveChilds
)
