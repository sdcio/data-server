package netconf

import (
	"fmt"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// TransformationContext
type TransformationContext struct {
	// LeafList contains the leaflist of the actual hierarchy level
	// it is to be converted into an schemapb.Update on existing the level
	leafLists map[string][]*schemapb.TypedValue
	pelems    []*schemapb.PathElem
}

func NewTransformationContext(pelems []*schemapb.PathElem) *TransformationContext {
	return &TransformationContext{
		pelems: pelems,
	}
}

// Close when closing the context, updates for LeafLists will be calculated and returned
func (tc *TransformationContext) Close() []*schemapb.Update {
	result := []*schemapb.Update{}
	// process LeafList Elements
	for k, v := range tc.leafLists {

		pathElems := append(tc.pelems, &schemapb.PathElem{Name: k})

		u := &schemapb.Update{
			Path: &schemapb.Path{
				Elem: pathElems,
			},
			Value: &schemapb.TypedValue{
				Value: &schemapb.TypedValue_LeaflistVal{
					LeaflistVal: &schemapb.ScalarArray{
						Element: v,
					},
				},
			},
		}
		result = append(result, u)
	}
	return result
}

func (tc *TransformationContext) String() (result string, err error) {
	result = "TransformationContext\n"
	for k, v := range tc.leafLists {
		vals := []string{}
		for _, val := range v {
			sval, err := valueAsString(val)
			if err != nil {
				return "", err
			}
			vals = append(vals, sval)
		}
		result += fmt.Sprintf("k: %s [%s]\n", k, strings.Join(vals, ", "))
	}
	return result, nil
}

func (tc *TransformationContext) AddLeafListEntry(name string, val *schemapb.TypedValue) error {

	// we do not expect the leafLists to be excesively in use, so we do late initialization
	// although the check is performed on every call to this function
	if tc.leafLists == nil {
		tc.leafLists = map[string][]*schemapb.TypedValue{}
	}

	var exists bool
	// add the tv array if it is the first element and hence does not exist
	if _, exists = tc.leafLists[name]; !exists {
		tc.leafLists[name] = []*schemapb.TypedValue{}
	}
	// append the tv to the list
	tc.leafLists[name] = append(tc.leafLists[name], val)
	return nil
}
