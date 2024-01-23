package jsonbuilder

import (
	"encoding/json"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
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
