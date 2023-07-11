package netconf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/iptecharch/data-server/datastore/target/netconf/conversion"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

func getNamespaceFromGetSchemaResponse(sr *sdcpb.GetSchemaResponse) string {
	switch sr.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return sr.Schema.GetContainer().GetNamespace()
	case *sdcpb.SchemaElem_Field:
		return sr.Schema.GetField().GetNamespace()
	case *sdcpb.SchemaElem_Leaflist:
		return sr.Schema.GetLeaflist().GetNamespace()
	}
	return ""
}

func valueAsString(v *sdcpb.TypedValue) (string, error) {
	switch v.Value.(type) {
	case *sdcpb.TypedValue_StringVal:
		return v.GetStringVal(), nil
	case *sdcpb.TypedValue_IntVal:
		return fmt.Sprintf("%d", v.GetIntVal()), nil
	case *sdcpb.TypedValue_UintVal:
		return fmt.Sprintf("%d", v.GetUintVal()), nil
	case *sdcpb.TypedValue_BoolVal:
		return string(strconv.FormatBool(v.GetBoolVal())), nil
	case *sdcpb.TypedValue_BytesVal:
		return string(v.GetBytesVal()), nil
	case *sdcpb.TypedValue_FloatVal:
		return string(strconv.FormatFloat(float64(v.GetFloatVal()), 'b', -1, 32)), nil
	// case *sdcpb.TypedValue_DecimalVal:
	// 	return fmt.Sprintf("%d", v.GetDecimalVal().Digits), nil
	case *sdcpb.TypedValue_AsciiVal:
		return v.GetAsciiVal(), nil
	case *sdcpb.TypedValue_LeaflistVal:
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
	case *sdcpb.TypedValue_AnyVal:
		return string(v.GetAnyVal().Value), nil
	case *sdcpb.TypedValue_JsonVal:
		return string(v.GetJsonVal()), nil
	case *sdcpb.TypedValue_JsonIetfVal:
		return string(v.GetJsonIetfVal()), nil
	case *sdcpb.TypedValue_ProtoBytes:
		return "PROTOBYTES", nil
	}
	return "", fmt.Errorf("TypedValue to String failed")
}

func StringElementToTypedValue(s string, ls *sdcpb.LeafSchema) (*sdcpb.TypedValue, error) {
	return conversion.Convert(s, ls.Type)
}

func pathElem2Xpath(pe *sdcpb.PathElem, namespace string) (etree.Path, error) {
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
