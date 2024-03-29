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

package jsonbuilder

import (
	"encoding/json"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type JsonBuilder struct {
	root *JEntry
}

func NewJsonBuilder() *JsonBuilder {
	return &JsonBuilder{
		root: NewJEntryMap("root"),
	}
}

func (jb *JsonBuilder) AddValue(p []*sdcpb.PathElem, v string) error {
	var err error
	actual_elem := jb.root
	var nextKeys map[string]string

	for idx, pe := range p {
		// fmt.Printf("Elem %s - %v\n", pe.Name, idx >= len(p)-1)
		// access the keys of the next element
		// they are required to figure out the type of
		// the to be created subelement
		if idx < len(p)-1 {
			nextKeys = p[idx+1].Key
		} else {
			nextKeys = nil
		}

		// we also indicate if this is the last element in the path
		actual_elem, err = actual_elem.GetSubElem(pe.Name, pe.Key, true, nextKeys, idx >= len(p)-1)
		if err != nil {
			return err
		}
	}
	actual_elem.AddToMap(p[len(p)-1].Name, NewJEntryString(v))

	return nil
}

func (jb *JsonBuilder) GetDoc() ([]byte, error) {
	return json.Marshal(jb.root)
}

func (jb *JsonBuilder) GetDocIndent() ([]byte, error) {
	return json.MarshalIndent(jb.root, "", "  ")
}
