package jsonbuilder

import (
	"encoding/json"
	"fmt"

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

	var prev_key map[string]string
	for _, pe := range p {
		fmt.Printf("Elem %s\n", pe.Name)
		actual_elem, err = actual_elem.GetSubElem(pe.Name, prev_key, len(pe.Key) > 0, true)
		if err != nil {
			return err
		}
		prev_key = pe.Key
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
