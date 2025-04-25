package tree

import (
	"math"
)

type choiceResolvers map[string]*choiceResolver

// AddChoice adds / registers a new Choice to the choiceCasesResolver
func (c choiceResolvers) AddChoice(name string) *choiceResolver {
	r := newChoiceResolver()
	c[name] = r
	return r
}

// GetDeletes returns the names of the elements that need to be deleted.
func (c choiceResolvers) GetDeletes() []string {
	result := []string{}
	for _, cases := range c {
		result = append(result, cases.GetDeletes()...)
	}
	return result
}

func (c choiceResolvers) deepCopy() choiceResolvers {
	result := choiceResolvers{}

	for k, v := range c {
		result[k] = v.deepCopy()
	}

	return result

}

// GetSkipElements returns the list of all choices elements that are not highes priority.
// The resulting slice is used to skip these elements.
func (c choiceResolvers) GetSkipElements() []string {
	result := []string{}
	for _, x := range c {
		result = append(result, x.GetSkipElements()...)
	}
	return result
}

// choiceResolver is a helper used to efficiently store the priority values of certain branches and their association to the cases is a choice.
// All with the goal of composing a list of elements that do not belong to the prioritised case, for exclusion on tree traversal time.
type choiceResolver struct {
	cases                map[string]*choicesCase
	elementToCaseMapping map[string]*choicesCase
	elements             map[string]*caseElement
}

func (c *choiceResolver) deepCopy() *choiceResolver {
	result := &choiceResolver{
		cases:                map[string]*choicesCase{},
		elementToCaseMapping: map[string]*choicesCase{},
		elements:             map[string]*caseElement{},
	}

	for k, v := range c.cases {
		result.cases[k] = v.deepCopy()
	}

	for _, cas := range result.cases {
		for _, elem := range cas.elements {
			result.elementToCaseMapping[elem.name] = cas
			result.elements[elem.name] = elem
		}
	}

	return result
}

// newChoiceResolver returns a ready to use choiceCasesResolver.
func newChoiceResolver() *choiceResolver {
	return &choiceResolver{
		cases:                map[string]*choicesCase{}, // case name -> case data
		elementToCaseMapping: map[string]*choicesCase{}, // element name -> case name
		elements:             map[string]*caseElement{}, //element name -> element data
	}
}

// AddCase adds / registers a case with its name and the element names that belong to the case
func (c *choiceResolver) AddCase(name string, elements []string) *choicesCase {
	cse := newChoicesCase(name)
	c.cases[name] = cse

	for _, e := range elements {
		cce := newCaseElement(e)
		c.elementToCaseMapping[e] = cse
		cse.elements[e] = cce
		c.elements[e] = cce
	}
	return cse
}

// SetValue Sets the priority value that the given elements with its entire branch has calculated
func (c *choiceResolver) SetValue(elemName string, highestWODel int32, highestDelete int32, highestWONew int32, isDeleted bool) {
	elem := c.elements[elemName]
	elem.highestWODeleted = highestWODel
	elem.highestWONew = highestWONew
}

// GetSkipElements returns the names of all the elements that belong to
// cases that have not the best priority
func (c *choiceResolver) GetSkipElements() []string {
	result := make([]string, 0, len(c.elementToCaseMapping))

	bestCase := c.GetBestCaseNow()
	for elem, cas := range c.elementToCaseMapping {
		if cas == bestCase {
			continue
		}
		result = append(result, elem)
	}
	return result
}

func (c *choiceResolver) GetBestCaseNow() *choicesCase {
	// best case now
	var highestCaseWODeleted *choicesCase
	highesPrecedenceWODeletedValue := int32(math.MaxInt32)

	for _, cas := range c.cases {
		actualCaseWODeletedValue := cas.getHighestPrecedenceWODeleted()
		if actualCaseWODeletedValue < highesPrecedenceWODeletedValue {
			highesPrecedenceWODeletedValue = actualCaseWODeletedValue
			highestCaseWODeleted = cas
		}
	}
	return highestCaseWODeleted
}

func (c *choiceResolver) GetBestCaseBefore() *choicesCase {
	// best case before
	var highestCaseWONew *choicesCase
	highesPrecedenceWONewValue := int32(math.MaxInt32)

	for _, cas := range c.cases {
		actualCaseWONewValue := cas.getHighestPrecedenceWONew()
		if actualCaseWONewValue < highesPrecedenceWONewValue {
			highesPrecedenceWONewValue = actualCaseWONewValue
			highestCaseWONew = cas
		}
	}
	return highestCaseWONew
}

func (c *choiceResolver) GetDeletes() []string {
	result := []string{}
	// best case now
	highestCaseNow := c.GetBestCaseNow()
	// best case before
	highestCaseBefore := c.GetBestCaseBefore()

	// if best case before (highestCaseWONew) != best case now (highestCaseWODeleted)
	if highestCaseBefore != nil && highestCaseBefore != highestCaseNow {
		// remove best case before
		result = append(result, highestCaseBefore.GetElementNames()...)
	}

	return result
}

// GetElementNames retrieve all the Element names involved in the Choice
func (c *choiceResolver) GetElementNames() []string {
	result := make([]string, 0, len(c.elementToCaseMapping))
	for elemName := range c.elementToCaseMapping {
		result = append(result, elemName)
	}
	return result
}

// choicesCase is the representation of a case in the choiceCasesResolver.
type choicesCase struct {
	name     string
	elements map[string]*caseElement
}

func newChoicesCase(name string) *choicesCase {
	return &choicesCase{
		name:     name,
		elements: map[string]*caseElement{},
	}
}

func (c *choicesCase) GetElementNames() []string {
	result := make([]string, 0, len(c.elements))
	for name := range c.elements {
		result = append(result, name)
	}
	return result
}

func (c *choicesCase) getHighestPrecedenceWONew() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if elem.highestWONew < result {
			result = elem.highestWONew
		}
	}
	return result
}

func (c *choicesCase) getHighestPrecedenceWODeleted() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if elem.highestWODeleted < result {
			result = elem.highestWODeleted
		}
	}
	return result
}

func (c *choicesCase) deepCopy() *choicesCase {
	result := &choicesCase{
		name:     c.name,
		elements: make(map[string]*caseElement),
	}
	for k, v := range c.elements {
		result.elements[k] = v.deepCopy()
	}
	return result
}

type caseElement struct {
	name             string
	highestWODeleted int32
	highestWONew     int32
}

func newCaseElement(name string) *caseElement {
	return &caseElement{
		name: name,
	}
}

func (c *caseElement) deepCopy() *caseElement {
	return &caseElement{
		name:             c.name,
		highestWODeleted: c.highestWODeleted,
		highestWONew:     c.highestWONew,
	}
}
