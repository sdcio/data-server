package utils

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func GetSchemaElemModuleName(s *sdcpb.SchemaElem) (moduleName string) {
	switch x := s.Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return x.Container.GetModuleName()
	case *sdcpb.SchemaElem_Leaflist:
		return x.Leaflist.GetModuleName()
	case *sdcpb.SchemaElem_Field:
		return x.Field.GetModuleName()
	}
	return ""
}

func GetNamespaceFromGetSchema(s *sdcpb.SchemaElem) string {
	switch s.GetSchema().(type) {
	case *sdcpb.SchemaElem_Container:
		return s.GetContainer().GetNamespace()
	case *sdcpb.SchemaElem_Field:
		return s.GetField().GetNamespace()
	case *sdcpb.SchemaElem_Leaflist:
		return s.GetLeaflist().GetNamespace()
	}
	return ""
}
