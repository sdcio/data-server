package schema

import (
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/goyang/pkg/yang"
)

func leafFromYEntry(e *yang.Entry) *schemapb.LeafSchema {
	l := &schemapb.LeafSchema{
		Name:           e.Name,
		Description:    e.Description,
		Owner:          "",
		Namespace:      e.Namespace().Name,
		Type:           toSchemaType(e.Type),
		IsMandatory:    e.Mandatory.Value(),
		Units:          e.Units,
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
	}
	if v, ok := e.SingleDefaultValue(); ok {
		l.Default = v
	}
	if e.Prefix != nil {
		l.Prefix = e.Prefix.Name
	}
	// if mustStatements, ok := e.Extra["must"]; ok {
	// 	for _, m := range mustStatements {
	// 		if m, ok := m.(*yang.Must); ok {
	// 			l.MustStatements = append(l.MustStatements, &schemapb.MustStatement{
	// 				Statement: m.Name,
	// 				Error:     m.ErrorMessage.Name,
	// 			})
	// 		}

	// 	}
	// }
	return l
}

func toSchemaType(yt *yang.YangType) *schemapb.SchemaLeafType {
	var values []string
	// yt := e.Type
	if yt.Enum != nil {
		values = yt.Enum.Names()
	}
	slt := &schemapb.SchemaLeafType{
		Type:       yang.TypeKind(yt.Kind).String(),
		Range:      yt.Range.String(),
		Values:     values,
		Units:      yt.Units,
		TypeName:   yt.Name,
		Leafref:    yt.Path,
		Patterns:   []*schemapb.SchemaPattern{},
		UnionTypes: []*schemapb.SchemaLeafType{},
	}
	for _, pat := range yt.Pattern {
		slt.Patterns = append(slt.Patterns, &schemapb.SchemaPattern{
			Pattern:  pat,
			Inverted: false,
		})
	}
	if yang.TypeKind(yt.Kind) == yang.Yunion {
		for _, ytt := range yt.Type {
			slt.UnionTypes = append(slt.UnionTypes, toSchemaType(ytt))
		}
	}
	if yang.TypeKind(yt.Kind) == yang.Yidentityref {
		for _, idBase := range yt.IdentityBase.Values {
			slt.Values = append(slt.Values, idBase.Name)
		}
	}
	return slt
}

func getMustStatement(e *yang.Entry) []*schemapb.MustStatement {
	mustStatements, ok := e.Extra["must"]
	if !ok {
		return nil
	}
	rs := make([]*schemapb.MustStatement, 0, len(mustStatements))
	for _, m := range mustStatements {
		if m, ok := m.(*yang.Must); ok {
			ms := &schemapb.MustStatement{
				Statement: m.Name,
			}
			if m.ErrorMessage != nil {
				ms.Error = m.ErrorMessage.Name
			}
			rs = append(rs, ms)
		}
	}
	return rs
}
