package netconf

import (
	"fmt"
	"strconv"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"

	"github.com/beevik/etree"
)

func getNamespaceFromGetSchemaResponse(sr *schemapb.GetSchemaResponse) (namespace string) {
	switch sr.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		namespace = sr.GetContainer().Namespace
	case *schemapb.GetSchemaResponse_Field:
		namespace = sr.GetField().Namespace
	case *schemapb.GetSchemaResponse_Leaflist:
		namespace = sr.GetLeaflist().Namespace
	}
	return namespace
}

func valueAsString(v *schemapb.TypedValue) (string, error) {

	switch v.Value.(type) {
	case *schemapb.TypedValue_StringVal:
		return v.GetStringVal(), nil
	case *schemapb.TypedValue_IntVal:
		return fmt.Sprintf("%d", v.GetIntVal()), nil
	case *schemapb.TypedValue_UintVal:
		return fmt.Sprintf("%d", v.GetUintVal()), nil
	case *schemapb.TypedValue_BoolVal:
		return string(strconv.FormatBool(v.GetBoolVal())), nil
	case *schemapb.TypedValue_BytesVal:
		return string(v.GetBytesVal()), nil
	case *schemapb.TypedValue_FloatVal:
		return string(strconv.FormatFloat(float64(v.GetFloatVal()), 'b', -1, 32)), nil
	case *schemapb.TypedValue_DecimalVal:
		return fmt.Sprintf("%d", v.GetDecimalVal().Digits), nil
	case *schemapb.TypedValue_AsciiVal:
		return v.GetAsciiVal(), nil
	case *schemapb.TypedValue_LeaflistVal:
	case *schemapb.TypedValue_AnyVal:
	case *schemapb.TypedValue_JsonVal:
	case *schemapb.TypedValue_JsonIetfVal:
	case *schemapb.TypedValue_ProtoBytes:
	}
	return "", fmt.Errorf("TypedValue to String failed")
}

func pathElem2Xpath(pe *schemapb.PathElem, namespace string) (etree.Path, error) {
	var keys []string

	// prepare the keys -> "k=v"
	for k, v := range pe.Key {
		keys = append(keys, fmt.Sprintf("%s='%s'", k, v))
	}

	keyString := ""
	if len(keys) > 0 {
		// join multiple key elements via comma
		keyString = "[" + strings.Join(keys, ",") + "]"
	}

	name := pe.Name
	//name := toNamespacedName(pe.Name, namespace)

	// build the final xpath
	filterString := fmt.Sprintf("./%s%s", name, keyString)
	return etree.CompilePath(filterString)
}
