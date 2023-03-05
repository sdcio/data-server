package schema

import (
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/goyang/pkg/yang"
)

func leafListFromYEntry(e *yang.Entry, withDesc bool) *schemapb.LeafListSchema {
	ll := &schemapb.LeafListSchema{
		Name: e.Name,
		// Description:    e.Description,
		Owner:          "",
		Namespace:      e.Namespace().Name,
		Type:           toSchemaType(e.Type),
		Units:          e.Units,
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
		IsUserOrdered:  false,
	}
	if withDesc {
		ll.Description = e.Description
	}
	if e.ListAttr != nil {
		ll.MaxElements = e.ListAttr.MaxElements
		ll.MinElements = e.ListAttr.MinElements
		if e.ListAttr.OrderedBy != nil {
			ll.IsUserOrdered = e.ListAttr.OrderedBy.Name == "user"
		}
	}
	if e.Prefix != nil {
		ll.Prefix = e.Prefix.Name
	}
	return ll
}
