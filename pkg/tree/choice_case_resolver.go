package tree

import "slices"

type choiceCasesResolvers map[string]*choiceCasesResolver

// AddChoice adds / registers a new Choice to the choiceCasesResolver
func (c choiceCasesResolvers) AddChoice(name string) *choiceCasesResolver {
	r := newChoiceCasesResolver()
	c[name] = r
	return r
}

// GetSkipElements returns the list of all choices elements that are not highes priority.
// The resulting slice is used to skip these elements.
func (c choiceCasesResolvers) GetSkipElements() []string {
	result := []string{}
	for _, x := range c {
		result = append(result, x.GetSkipElements()...)
	}
	return result
}

// GetChoiceElements returns a list of elements that belong to the same choice
// as the given element. This is used to query the cache for all elements of all cases for the
// choice.
func (c choiceCasesResolvers) GetChoiceElementNeighbors(elemName string) []string {
	var result []string
	// iterate through the different choices that might exist
	for _, choice := range c {
		// get all the elements references in a certain contains
		sl := choice.GetElementNames()
		// check for the given element, if it belongs to the actual choice
		var idx int
		if idx = slices.Index(sl, elemName); idx == -1 {
			// if not continue with next choice
			continue
		}
		// if it belongs to it, return the list of all elements belonging to the
		// choice without the provided element itself.
		return append(sl[:idx], sl[idx+1:]...)

	}
	return result
}

// choiceCasesResolver is a helper used to efficiently store the priority values of certain branches and their association to the cases is a choice.
// All with the goal of composing a list of elements that do not belong to the prioritised case, for exclusion on tree traversal time.
type choiceCasesResolver struct {
	cases                map[string]*choicesCase
	bestcase             *string
	elementToCaseMapping map[string]string
}

// GetElementNames retrieve all the Element names involved in the Choice
func (c *choiceCasesResolver) GetElementNames() []string {
	result := make([]string, 0, len(c.cases))
	for k := range c.cases {
		result = append(result, k)
	}
	return result
}

// choicesCase is the representation of a case in the choiceCasesResolver.
type choicesCase struct {
	name     string
	value    *int32
	elements []string
}

// newChoiceCasesResolver returns a ready to use choiceCasesResolver.
func newChoiceCasesResolver() *choiceCasesResolver {
	return &choiceCasesResolver{
		cases:                map[string]*choicesCase{},
		elementToCaseMapping: map[string]string{},
	}
}

// AddCase adds / registers a case with its name and the element names that belong to the case
func (c *choiceCasesResolver) AddCase(name string, elements []string) *choicesCase {
	c.cases[name] = &choicesCase{
		name:     name,
		elements: elements,
	}
	for _, e := range elements {
		c.elementToCaseMapping[e] = name
	}
	return c.cases[name]
}

// SetValue Sets the priority value that the given elements with its entire branch has calculated
func (c *choiceCasesResolver) SetValue(elemName string, v int32) {
	actualCase := c.elementToCaseMapping[elemName]
	elem := c.cases[actualCase]
	if elem.value == nil {
		elem.value = &v
	}
	if c.bestcase == nil {
		c.bestcase = &actualCase
		return
	}
	if v < *c.cases[*c.bestcase].value {
		c.bestcase = &actualCase
	}
}

// GetBestCaseName returns the name of the case, that has the highes priority
func (c *choiceCasesResolver) GetBestCaseName() string {
	return *c.bestcase
}

// GetSkipElements returns the names of all the elements that belong to
// cases that have not the best priority
func (c *choiceCasesResolver) GetSkipElements() []string {
	result := make([]string, 0, len(c.elementToCaseMapping))
	for elem, cas := range c.elementToCaseMapping {
		if cas == *c.bestcase {
			continue
		}
		// optimization, add to the skip list only items that do exist.
		if c.cases[cas].value == nil {
			continue
		}
		result = append(result, elem)
	}
	return result
}
