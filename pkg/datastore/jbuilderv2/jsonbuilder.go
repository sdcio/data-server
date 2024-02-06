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

package jbuilderv2

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	schemaClient "github.com/iptecharch/data-server/pkg/datastore/clients/schema"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

const (
	stringType      = "string"
	int8Type        = "int8"
	int16Type       = "int16"
	int32Type       = "int32"
	int64Type       = "int64"
	uint8Type       = "uint8"
	uint16Type      = "uint16"
	uint32Type      = "uint32"
	uint64Type      = "uint64"
	floatType       = "float"
	boolType        = "boolean"
	nullType        = "null"
	presenceType    = "PRESENCE"
	decimal64Type   = "decimal64"
	leafrefType     = "leafref"
	enumType        = "enumeration"
	unionType       = "union"
	identityrefType = "identityref"
)

type jsonBuilder struct {
	scc *schemaClient.SchemaClientBound
}

// pathElem is a path element that carries the keys types as well as their values
type pathElem struct {
	name         string
	keyValueType map[string]*vt
}

type vt struct {
	value  string
	conVal any
}

func (pe *pathElem) String() string {
	numKeys := len(pe.keyValueType)
	if numKeys == 0 {
		return pe.name
	}

	sb := &strings.Builder{}
	sb.WriteString(pe.name)

	keys := make([]string, 0, numKeys)
	for k := range pe.keyValueType {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(sb, "[%s=%s:%T]", k, pe.keyValueType[k].value, pe.keyValueType[k].conVal)
	}
	return sb.String()
}

func New(scc *schemaClient.SchemaClientBound) *jsonBuilder {
	return &jsonBuilder{
		scc: scc,
	}
}

// AddUpdate adds the path and value to the obj map.
// It does that by checking the schema of the object the path points to.
// It converts keys along the path to their YANG type.
// It ignores non-presence containers.
// If the path points to a leaf-list. The value is expected to be a TypedValue_leafList.
// If the path points to a field (i.e leaf), the typedValue should not be nil and is expected to reflect its YANG type.
func (j *jsonBuilder) AddUpdate(ctx context.Context, obj map[string]any, p *sdcpb.Path, tv *sdcpb.TypedValue) error {
	psc, err := j.scc.GetSchema(ctx, p)
	if err != nil {
		return err
	}

	if isDefault(tv, psc.GetSchema()) && tv != nil && tv.GetValue() != nil {
		return nil
	}
	value := getValue(tv)

	switch {
	case psc.GetSchema().GetContainer() != nil:
		if !psc.GetSchema().GetContainer().GetIsPresence() {
			return nil
		}
		value = map[string]any{}
	}

	pes, err := j.buildPathElems(ctx, p)
	if err != nil {
		return err
	}

	err = j.addValueToObject(obj, pes, value)
	if err != nil {
		return err
	}
	return nil
}

// convertKeyValue converts a string value to the specified type.
func convertKeyValue(value, valueType string) (interface{}, error) {
	switch valueType {
	case stringType:
		return value, nil
	case int8Type, int16Type, int32Type, uint8Type, uint16Type, uint32Type:
		return strconv.Atoi(value)
	case floatType:
		return strconv.ParseFloat(value, 64)
	case boolType:
		return strconv.ParseBool(value)
	case nullType:
		return nil, nil
	case presenceType:
		return map[string]any{}, nil
	case decimal64Type:
		return value, nil
	case "":
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported value type: %q", valueType)
	}
}

// addValueToObject adds a value to the JSON object at the specified path.
func (j *jsonBuilder) addValueToObject(obj map[string]any, path []*pathElem, value interface{}) error {
	pLen := len(path)
	for i, elem := range path {
		// Check if it's the last element
		if i == pLen-1 {
			switch value.(type) {
			case []any:
				switch obj[elem.name].(type) {
				case []any: // YANG leaf-list case
					obj[elem.name] = append(obj[elem.name].([]any), value.([]any)...)
				default: // YANG leaf or presence container case
					obj[elem.name] = value
				}
			default:
				obj[elem.name] = value
			}
			continue
		}
		// Handle nested objects or arrays i.e YANG containers
		if len(elem.keyValueType) == 0 {
			// If no keys, ensure the next element is a nested object
			if _, ok := obj[elem.name]; !ok {
				obj[elem.name] = make(map[string]interface{})
			}
			switch obj[elem.name].(type) {
			case map[string]interface{}:
				obj = obj[elem.name].(map[string]interface{})
			default:
				log.Warnf("json_builder: unexpected element type at %s: got %T", elem.name, obj[elem.name])
				log.Warnf("json_builder: path %s | value %T | %v", path, value, value)
			}
			continue
		}
		// Handle keys for arrays of objects i.e YANG lists
		if _, ok := obj[elem.name]; !ok {
			obj[elem.name] = make([]map[string]interface{}, 0)
		}

		arr, ok := obj[elem.name].([]map[string]interface{})
		if !ok {
			return fmt.Errorf("failed: path element '%s' is not an array of objects as expected: %T", elem.name, obj[elem.name])
		}

		matchedObj, found, err := findOrCreateMatchingObject(arr, elem.keyValueType)
		if err != nil {
			return err
		}
		if !found {
			arr = append(arr, matchedObj)
			obj[elem.name] = arr
		}
		obj = matchedObj
	}
	return nil
}

// findOrCreateMatchingObject finds or creates an object that matches the key-value pairs.
// i.e this function creates items under a list given a set of keys.
// It does that by looping through the maps in the given array looking for a map
// that has the same keys/values as the provided map 'keys'.
// If it finds a match, it returns it.
// If it doesn't, it creates the map and populates it with
// the provided keys.
func findOrCreateMatchingObject(arr []map[string]interface{}, keys map[string]*vt) (map[string]interface{}, bool, error) {
	for _, obj := range arr {
		match := true
		for k, v := range keys {
			if !reflect.DeepEqual(obj[k], v.conVal) {
				match = false
				break
			}
		}
		if match {
			return obj, true, nil
		}
	}

	// No match found, create a new object
	newObj := make(map[string]interface{})
	for k, v := range keys {
		newObj[k] = v.conVal
	}
	return newObj, false, nil
}

func (j *jsonBuilder) buildPathElems(ctx context.Context, p *sdcpb.Path) ([]*pathElem, error) {
	pes := make([]*pathElem, 0, len(p.GetElem()))
	for i, sdcpe := range p.GetElem() {
		pe := &pathElem{
			name:         sdcpe.GetName(),
			keyValueType: map[string]*vt{},
		}
		for k, v := range sdcpe.GetKey() {
			pe.keyValueType[k] = &vt{value: v}
		}

		basePath := &sdcpb.Path{Elem: make([]*sdcpb.PathElem, 0, i)}
		for j := 0; j <= i; j++ {
			basePath.Elem = append(basePath.Elem, &sdcpb.PathElem{Name: p.GetElem()[j].GetName()})
		}

		// check if basePath points to a list.
		baseListSchema, err := j.scc.GetSchema(ctx, basePath)
		if err != nil {
			return nil, err
		}

		switch {
		case baseListSchema.GetSchema().GetContainer() != nil:
			// convert keys
			for _, keySchema := range baseListSchema.GetSchema().GetContainer().GetKeys() {
				val := pe.keyValueType[keySchema.GetName()].value
				typ := toBasicType(keySchema.GetType(), val)
				convVal, err := convertKeyValue(val, typ)
				if err != nil {
					return nil, err
				}
				pe.keyValueType[keySchema.GetName()].conVal = convVal
			}
		}
		pes = append(pes, pe)
	}
	return pes, nil
}

func toBasicType(sclt *sdcpb.SchemaLeafType, val string) string {
	switch sclt.Type {
	case int64Type, uint64Type, identityrefType, enumType: // TODO: revisit enum type
		return stringType
	case unionType:
		return getTypeUnion(sclt, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: val}})
	case "leafref":
		// TODO: follow leaf ref and return its type
		//       strip the leafref path from ns and query its schema
		return stringType
	default:
		return sclt.Type
	}
}

func getTypeUnion(uType *sdcpb.SchemaLeafType, tv *sdcpb.TypedValue) string {
	for _, ut := range uType.GetUnionTypes() {
		_, err := convertKeyValue(tv.GetStringVal(), ut.GetType())
		if err == nil {
			return ut.GetType()
		}
	}
	// default to string
	return stringType
}

func isDefault(tv *sdcpb.TypedValue, schemaElem *sdcpb.SchemaElem) bool {
	defaultVal := ""
	switch {
	case schemaElem.GetContainer() != nil:
		return false
	case schemaElem.GetLeaflist() != nil:
		lfDefaultVals := schemaElem.GetLeaflist().GetDefaults()
		numDefaults := len(lfDefaultVals)
		switch tv.Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			numlf := len(tv.GetLeaflistVal().GetElement())
			if numlf != numDefaults {
				return false
			}
			for i, lftv := range tv.GetLeaflistVal().GetElement() {
				if !tvIsEqual(lftv, lfDefaultVals[i]) {
					return false
				}
			}
			return true
		default:
			// if there are no defaults or there
			// are more than one, return false
			if numDefaults != 1 {
				return false
			}
			// compare a single leaf-list as a leaf
			defaultVal = lfDefaultVals[0]
		}
	case schemaElem.GetField() != nil:
		defaultVal = schemaElem.GetField().GetDefault()
	}

	return tvIsEqual(tv, defaultVal)
}

func tvIsEqual(tv *sdcpb.TypedValue, val string) bool {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_AsciiVal:
		return tv.GetAsciiVal() == val
	case *sdcpb.TypedValue_BoolVal:
		v, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}
		return tv.GetBoolVal() == v
	case *sdcpb.TypedValue_BytesVal:
		// TODO: recheck
		return bytes.Equal([]byte(val), tv.GetBytesVal())
	case *sdcpb.TypedValue_DecimalVal:
		vf, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		dem := math.Pow10(int(tv.GetDecimalVal().GetPrecision()))
		if dem == 0 {
			return false
		}
		num := float64(tv.GetDecimalVal().GetDigits())
		f := num / dem
		return vf == f
	case *sdcpb.TypedValue_FloatVal:
		v, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return false
		}
		return tv.GetFloatVal() == float32(v)
	case *sdcpb.TypedValue_DoubleVal:
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return tv.GetDoubleVal() == v
	case *sdcpb.TypedValue_IntVal:
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return false
		}
		return tv.GetIntVal() == v
	case *sdcpb.TypedValue_StringVal:
		return tv.GetStringVal() == val
	case *sdcpb.TypedValue_UintVal:
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return false
		}
		return tv.GetUintVal() == uint64(v)
	case *sdcpb.TypedValue_JsonIetfVal:
	case *sdcpb.TypedValue_JsonVal:
	case *sdcpb.TypedValue_LeaflistVal: // should not reach here
	case *sdcpb.TypedValue_ProtoBytes:
	case *sdcpb.TypedValue_AnyVal:
	}
	return false
}

func getValue(tv *sdcpb.TypedValue) any {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_AsciiVal:
		return tv.GetAsciiVal()
	case *sdcpb.TypedValue_BoolVal:
		return tv.GetBoolVal()
	case *sdcpb.TypedValue_BytesVal:
		return tv.GetBytesVal()
	case *sdcpb.TypedValue_DecimalVal:
		return tv.GetDecimalVal()
	case *sdcpb.TypedValue_FloatVal:
		return tv.GetFloatVal()
	case *sdcpb.TypedValue_DoubleVal:
		return tv.GetDoubleVal()
	case *sdcpb.TypedValue_IntVal:
		return tv.GetIntVal()
	case *sdcpb.TypedValue_StringVal:
		return tv.GetStringVal()
	case *sdcpb.TypedValue_UintVal:
		return tv.GetUintVal()
	case *sdcpb.TypedValue_JsonIetfVal:
		return tv.GetJsonIetfVal()
	case *sdcpb.TypedValue_JsonVal:
		return tv.GetJsonVal()
	case *sdcpb.TypedValue_LeaflistVal:
		rs := make([]any, 0, len(tv.GetLeaflistVal().GetElement()))
		for _, e := range tv.GetLeaflistVal().GetElement() {
			rs = append(rs, getValue(e))
		}
		return rs
	case *sdcpb.TypedValue_ProtoBytes:
		return tv.GetProtoBytes()
	case *sdcpb.TypedValue_AnyVal:
		return tv.GetAnyVal()
	}
	return nil
}
