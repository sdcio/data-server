package schema

import (
	"sort"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/goyang/pkg/yang"
)

func containerFromYEntry(e *yang.Entry, withDesc bool) *schemapb.ContainerSchema {
	c := &schemapb.ContainerSchema{
		Name: e.Name,
		// Description:    e.Description,
		Owner:          "", // TODO:
		Namespace:      e.Namespace().Name,
		Keys:           []*schemapb.LeafSchema{},
		Fields:         []*schemapb.LeafSchema{},
		Leaflists:      []*schemapb.LeafListSchema{},
		Children:       []string{},
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
		IsPresence:     isPresence(e),
	}
	if withDesc {
		c.Description = e.Description
	}
	if e.ListAttr != nil {
		c.MaxElements = e.ListAttr.MaxElements
		c.MinElements = e.ListAttr.MinElements
		if e.ListAttr.OrderedBy != nil {
			c.IsUserOrdered = e.ListAttr.OrderedBy.Name == "user"
		}
	}
	if e.Prefix != nil {
		c.Prefix = e.Prefix.Name
	}

	keys := map[string]struct{}{}
	for _, key := range strings.Fields(e.Key) {
		keys[key] = struct{}{}
	}
	for _, child := range getChildren(e) {
		switch {
		case child.IsDir():
			c.Children = append(c.Children, child.Name)
		case child.IsLeaf(), child.IsLeafList():
			o := ObjectFromYEntry(child, withDesc)
			switch o := o.(type) {
			case *schemapb.LeafSchema:
				if _, ok := keys[child.Name]; ok {
					c.Keys = append(c.Keys, o)
					continue
				}
				c.Fields = append(c.Fields, o)
			case *schemapb.LeafListSchema:
				c.Leaflists = append(c.Leaflists, o)
			}
		}
	}
	sort.Strings(c.Children)
	return c
}

func isState(e *yang.Entry) bool {
	if e.Config == yang.TSFalse {
		return true
	}
	if e.Parent != nil {
		return isState(e.Parent)
	}
	return false
}

func isPresence(e *yang.Entry) bool {
	if presence, ok := e.Extra["presence"]; ok {
		for _, m := range presence {
			if _, ok := m.(*yang.Value); ok {
				return ok
			}
		}
		return false
	}
	return false
}
