package tree

import (
	"math"
	"slices"
	"strings"

	log "github.com/sirupsen/logrus"
)

type choiceResolvers map[string]*choiceResolver

// AddChoice adds / registers a new Choice to the choiceCasesResolver
func (c choiceResolvers) AddChoice(name string) *choiceResolver {
	r := newChoiceCasesResolver()
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

// GetChoiceElements returns a list of elements that belong to the same choice
// as the given element. This is used to query the cache for all elements of all cases for the
// choice.
func (c choiceResolvers) GetChoiceElementNeighbors(elemName string) []string {
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
	}

	for k, v := range c.cases {
		result.cases[k] = v.deepCopy()
	}

	for k, v := range c.elementToCaseMapping {
		result.elementToCaseMapping[k] = v
	}

	return result
}

// newChoiceCasesResolver returns a ready to use choiceCasesResolver.
func newChoiceCasesResolver() *choiceResolver {
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
func (c *choiceResolver) SetValue(elemName string, vWODel int32, vWDel int32, new bool, deleted bool) {
	elem := c.elements[elemName]
	elem.valueWODelete = vWODel
	elem.valueWDelete = vWDel
	elem.new = new
	elem.deleted = deleted
}

// GetBestCaseName returns the name of the case, that has the highes priority
func (c *choiceResolver) getBestCaseName() string {
	var bestCaseName string
	bestCasePrio := int32(math.MaxInt32)
	// we might have multiple cases that carry the same, the best priority which is an issue
	// to identify these circumstances, we curate this list that contain the names of the cases
	// that have the actual best priority
	equalBestCases := []string{}
	for caseName, cas := range c.cases {
		casLowest := cas.GetLowestPriorityValue()
		if casLowest <= bestCasePrio && casLowest < int32(math.MaxInt32) {
			// reset slice with equal bestcases if new bestcase is lower
			if casLowest < bestCasePrio {
				// reset alice without reallocation
				equalBestCases = equalBestCases[:0]
			}
			// add to best cases
			equalBestCases = append(equalBestCases, caseName)

			// update the best case
			bestCaseName = caseName
			bestCasePrio = casLowest
		}
	}
	// if we have multiple best cases with same priority, the whole result is flaky since it relies on map ieration order.
	if len(equalBestCases) > 1 {
		log.Warnf("Undecidable new best case [ %s ] all have prio %d, reporting %s as best now", strings.Join(equalBestCases, ", "), bestCasePrio, bestCaseName)
	}
	return bestCaseName
}

// GetSkipElements returns the names of all the elements that belong to
// cases that have not the best priority
func (c *choiceResolver) GetSkipElements() []string {
	result := make([]string, 0, len(c.elementToCaseMapping))

	bestCase := c.getBestCaseName()

	for elem, cas := range c.elementToCaseMapping {
		if cas.name == bestCase {
			continue
		}
		result = append(result, elem)
	}
	return result
}

func (c *choiceResolver) GetDeletes() []string {
	deletBased := getHighest(c.cases,
		func(cc *choicesCase) int32 {
			return cc.getHighestPrecedenceWDelete()
		},
		func(cc *choicesCase) int32 {
			return cc.getHighestPrecedenceWODelete()
		})

	newBased := getHighest(c.cases,
		func(cc *choicesCase) int32 {
			return cc.getHighestPrecedenceNonNew()
		},
		func(cc *choicesCase) int32 {
			return cc.getHighestPrecedenceNew()
		},
	)

	return append(deletBased, newBased...)
}

func getHighest(c map[string]*choicesCase, fWith func(*choicesCase) int32, fWithout func(*choicesCase) int32) []string {

	var highestCaseWith *choicesCase
	var highestCaseWithout *choicesCase

	highestCaseWithValue := int32(math.MaxInt32)
	highestCaseWithoutValue := int32(math.MaxInt32)

	for _, cse := range c {
		// get with delete highes prio value
		highesCasePrecedenceWith := fWith(cse)
		if highesCasePrecedenceWith < highestCaseWithValue {
			// update highes reference
			highestCaseWithValue = highesCasePrecedenceWith
			highestCaseWith = cse
		}

		// get without delete highes prio value
		highesCasePrecedenceWithout := fWithout(cse)
		if highesCasePrecedenceWithout < highestCaseWithoutValue {
			// update highes reference
			highestCaseWithoutValue = highesCasePrecedenceWithout
			highestCaseWithout = cse
		}
	}

	var result []string
	if highestCaseWithValue < math.MaxInt32 && highestCaseWith != highestCaseWithout {
		result = highestCaseWith.GetElementNames()
	}

	return result
}

func (c *choiceResolver) GetUpdate() {

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

func (c *choicesCase) containsNew() bool {
	for _, elem := range c.elements {
		if elem.new {
			return true
		}
	}
	return false
}

func (c *choicesCase) getHighestPrecedenceNew() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if elem.new && elem.valueWDelete < result {
			result = elem.valueWDelete
		}
	}
	return result
}

func (c *choicesCase) getHighestPrecedenceNonNew() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if !elem.new && elem.valueWDelete < result {
			result = elem.valueWDelete
		}
	}
	return result
}

func (c *choicesCase) getHighestPrecedenceWODelete() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if elem.valueWODelete < result {
			result = elem.valueWODelete
		}
	}
	return result
}

func (c *choicesCase) getHighestPrecedenceWDelete() int32 {
	result := int32(math.MaxInt32)
	for _, elem := range c.elements {
		if elem.valueWDelete < result {
			result = elem.valueWDelete
		}
	}
	return result
}

func (c *choicesCase) deepCopy() *choicesCase {
	result := &choicesCase{
		name: c.name,
	}
	for k, v := range c.elements {
		result.elements[k] = v.deepCopy()
	}
	return result
}

func (c *choicesCase) GetLowestPriorityValue() int32 {
	result := int32(math.MaxInt32)
	for _, cas := range c.elements {
		if cas.valueWODelete < result {
			result = cas.valueWODelete
		}
	}
	return result
}

func (c *choicesCase) GetLowestPriorityValueOld() int32 {
	result := int32(math.MaxInt32)
	for _, cas := range c.elements {
		if !cas.new && cas.valueWODelete < result {
			result = cas.valueWODelete
		}
	}
	return result
}

type caseElement struct {
	name          string
	valueWODelete int32
	valueWDelete  int32
	new           bool
	deleted       bool
}

func newCaseElement(name string) *caseElement {
	return &caseElement{
		name:          name,
		valueWODelete: int32(math.MaxInt32),
	}
}

func (c *caseElement) deepCopy() *caseElement {
	return &caseElement{
		name:          c.name,
		valueWODelete: c.valueWODelete,
		new:           c.new,
	}
}
