// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package netconf

import (
	"fmt"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// TransformationContext
type TransformationContext struct {
	// LeafList contains the leaflist of the actual hierarchy level
	// it is to be converted into an sdcpb.Update on existing the level
	leafLists map[string][]*sdcpb.TypedValue
	pelems    []*sdcpb.PathElem
}

func NewTransformationContext(pelems []*sdcpb.PathElem) *TransformationContext {
	return &TransformationContext{
		pelems: pelems,
	}
}

// Close when closing the context, updates for LeafLists will be calculated and returned
func (tc *TransformationContext) Close() []*sdcpb.Update {
	result := []*sdcpb.Update{}
	// process LeafList Elements
	for k, v := range tc.leafLists {

		pathElems := append(tc.pelems, &sdcpb.PathElem{Name: k})

		u := &sdcpb.Update{
			Path: &sdcpb.Path{
				Elem: pathElems,
			},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_LeaflistVal{
					LeaflistVal: &sdcpb.ScalarArray{
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

func (tc *TransformationContext) AddLeafListEntry(name string, val *sdcpb.TypedValue) error {

	// we do not expect the leafLists to be excesively in use, so we do late initialization
	// although the check is performed on every call to this function
	if tc.leafLists == nil {
		tc.leafLists = map[string][]*sdcpb.TypedValue{}
	}

	var exists bool
	// add the tv array if it is the first element and hence does not exist
	if _, exists = tc.leafLists[name]; !exists {
		tc.leafLists[name] = []*sdcpb.TypedValue{}
	}
	// append the tv to the list
	tc.leafLists[name] = append(tc.leafLists[name], val)
	return nil
}
