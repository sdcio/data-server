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

package netconf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/data-server/pkg/datastore/target/netconf/conversion"
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

// pathElem2EtreePath takes the given pathElem and creates an xpath expression out of it,
// which is then used to build an etree.Path, which is then returned
func pathElem2EtreePath(pe *sdcpb.PathElem) (etree.Path, error) {
	xpathString, err := pathElem2XPath(pe)
	if err != nil {
		return etree.Path{}, err
	}
	return etree.CompilePath(xpathString)
}

// pathElem2XPath takes the given PathElem and generates the corresponding xpath expression
func pathElem2XPath(pe *sdcpb.PathElem) (string, error) {
	keys := make([]string, 0, len(pe.GetKey()))
	// prepare the keys -> "k='v'"
	for k, v := range pe.Key {
		keys = append(keys, fmt.Sprintf("%s='%s'", k, v))
	}
	sort.Strings(keys)
	keyString := ""
	if len(keys) > 0 {
		// join multiple key elements via comma
		keyString = "[" + strings.Join(keys, ",") + "]"
	}

	// build the final xpath
	filterString := fmt.Sprintf("./%s%s", pe.Name, keyString)
	return filterString, nil
}
