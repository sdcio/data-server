package netconf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/iptecharch/schema-server/datastore/target/netconf/conversion"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

func getNamespaceFromGetSchemaResponse(sr *schemapb.GetSchemaResponse) string {
	switch sr.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		return sr.GetContainer().GetNamespace()
	case *schemapb.GetSchemaResponse_Field:
		return sr.GetField().GetNamespace()
	case *schemapb.GetSchemaResponse_Leaflist:
		return sr.GetLeaflist().GetNamespace()
	}
	return ""
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
	// case *schemapb.TypedValue_DecimalVal:
	// 	return fmt.Sprintf("%d", v.GetDecimalVal().Digits), nil
	case *schemapb.TypedValue_AsciiVal:
		return v.GetAsciiVal(), nil
	case *schemapb.TypedValue_LeaflistVal:
		sArr := []string{}
		// iterate through list elements
		for _, elem := range v.GetLeaflistVal().Element {
			val, err := valueAsString(elem)
			if err != nil {
				return "", err
			}
			sArr = append(sArr, val)
		}
		return fmt.Sprintf("[ %s ]", strings.Join(sArr, ", ")), nil
	case *schemapb.TypedValue_AnyVal:
		return string(v.GetAnyVal().Value), nil
	case *schemapb.TypedValue_JsonVal:
		return string(v.GetJsonVal()), nil
	case *schemapb.TypedValue_JsonIetfVal:
		return string(v.GetJsonIetfVal()), nil
	case *schemapb.TypedValue_ProtoBytes:
		return "PROTOBYTES", nil
	}
	return "", fmt.Errorf("TypedValue to String failed")
}

func StringElementToTypedValue(s string, ls *schemapb.LeafSchema) (*schemapb.TypedValue, error) {
	return conversion.Convert(s, ls.Type)
}

func pathElem2Xpath(pe *schemapb.PathElem, namespace string) (etree.Path, error) {
	var keys []string

	// prepare the keys -> "k=v"
	for k, v := range pe.Key {
		keys = append(keys, fmt.Sprintf("%s=%s", k, v))
	}

	keyString := ""
	if len(keys) > 0 {
		// join multiple key elements via comma
		keyString = "[" + strings.Join(keys, ",") + "]"
	}

	//name := toNamespacedName(pe.Name, namespace)

	// build the final xpath
	filterString := fmt.Sprintf("./%s%s", pe.Name, keyString)
	return etree.CompilePath(filterString)
}
