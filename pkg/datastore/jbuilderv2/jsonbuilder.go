package jbuilderv2

import (
	"context"
	"fmt"
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

	leafListTypePrefix = "LEAFLIST:"
)

type jsonBuilder struct {
	scc *schemaClient.SchemaClientBound
}

type pathElem struct {
	name         string
	keyValueType map[string]*vt // TODO: replace with sdcpb.TypedValue
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

type update struct {
	path  []*pathElem
	value string
	// convertedValue any // TODO: replace with sdcpb.TypedValue
}

func New(scc *schemaClient.SchemaClientBound) *jsonBuilder {
	return &jsonBuilder{
		scc: scc,
	}
}

// AddUpdate adds the path and value to the obj map.
// It does that by checking the schema of the object the path points to.
// It ignores non-presence containers.
// If the path points to a leaf-list. The value is expected to be nil and the value to be set will be the key
// in the path's last element.
// If the path points to a field (i.e leaf), the typedValue should not be nil and is expected to be (for now) a StringVal.
func (j *jsonBuilder) AddUpdate(ctx context.Context, obj map[string]any, p *sdcpb.Path, tv *sdcpb.TypedValue) error {
	psc, err := j.scc.GetSchema(ctx, p)
	if err != nil {
		return err
	}

	vt, isDefault, err := j.getType(ctx, tv, psc)
	if err != nil {
		return err
	}

	if isDefault && tv != nil && tv.GetValue() != nil {
		return nil
	}

	// u := update{valueType: vt}
	u := update{}
	switch {
	case psc.GetSchema().GetContainer() != nil:
		if !psc.GetSchema().GetContainer().IsPresence {
			return nil
		}
	case psc.GetSchema().GetLeaflist() != nil:
		for _, v := range p.GetElem()[len(p.GetElem())-1].GetKey() {
			u.value = v
			break
		}
	case psc.GetSchema().GetField() != nil:
		u.value = tv.GetStringVal()
	}

	u.path, err = j.buildPathElems(ctx, p)
	if err != nil {
		return err
	}

	convertedValue, err := convertValue(u.value, vt)
	if err != nil {
		return err
	}

	err = j.addValueToObject(obj, u.path, convertedValue)
	if err != nil {
		return err
	}
	return nil
}

// convertValue converts a string value to the specified type.
func convertValue(value, valueType string) (interface{}, error) {
	// check if the type is a leaflist type i.e LEAFLIST:XXXX
	// this check can be replaced with strings.Index if there is a performance issue.. unlikely.
	if strings.HasPrefix(valueType, leafListTypePrefix) {
		lfVal, err := convertValue(value, strings.TrimPrefix(valueType, leafListTypePrefix))
		if err != nil {
			return nil, err
		}
		// return as a []any so that it can be concatenated with
		// other values of the same leaf-list
		return []any{lfVal}, nil
	}
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
	// case "LEAFLIST": // not needed ?
	// 	return []any{value}, nil
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
				log.Warnf("unexpected element type at %s: got %T", elem.name, obj[elem.name])
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

func (j *jsonBuilder) getType(ctx context.Context, tv *sdcpb.TypedValue, rsp *sdcpb.GetSchemaResponse) (string, bool, error) {
	switch rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return presenceType, getDefault(rsp) == tv.GetStringVal(), nil
	case *sdcpb.SchemaElem_Leaflist:
		llType := getLeaflistType(rsp.GetSchema().GetLeaflist().GetType(), tv)
		return llType, getDefault(rsp) == tv.GetStringVal(), nil
	case *sdcpb.SchemaElem_Field:
		switch t := rsp.Schema.GetField().GetType().GetType(); t {
		case int64Type, uint64Type, enumType:
			return stringType, getDefault(rsp) == tv.GetStringVal(), nil
		case unionType:
			ut := getTypeUnion(rsp.Schema.GetField().GetType(), tv)
			return ut, getDefault(rsp) == tv.GetStringVal(), nil
		case identityrefType:
			return stringType, getDefault(rsp) == tv.GetStringVal(), nil
		case leafrefType:
			// TODO: query leafref schema and return its type
			return stringType, getDefault(rsp) == tv.GetStringVal(), nil
		default:
			return t, getDefault(rsp) == tv.GetStringVal(), nil
		}
	default:
		return "", getDefault(rsp) == tv.GetStringVal(), nil
	}
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

		// check if basePath points to a list or leaf-list.
		baseListSchema, err := j.scc.GetSchema(ctx, basePath)
		if err != nil {
			return nil, err
		}

		switch {
		case baseListSchema.GetSchema().GetContainer() != nil:
			for _, keySchema := range baseListSchema.GetSchema().GetContainer().GetKeys() {
				val := pe.keyValueType[keySchema.GetName()].value
				typ := toBasicType(keySchema.GetType(), val)
				convVal, err := convertValue(val, typ)
				if err != nil {
					return nil, err
				}
				pe.keyValueType[keySchema.GetName()].conVal = convVal
			}
		case baseListSchema.GetSchema().GetLeaflist() != nil:
			val := pe.keyValueType[sdcpe.GetName()].value
			typ := toBasicType(baseListSchema.GetSchema().GetLeaflist().GetType(), pe.keyValueType[sdcpe.GetName()].value)
			convVal, err := convertValue(val, typ)
			if err != nil {
				return nil, err
			}
			pe.keyValueType[sdcpe.GetName()].conVal = convVal
		}
		pes = append(pes, pe)
	}
	return pes, nil
}

func getDefault(rsp *sdcpb.GetSchemaResponse) string {
	if rsp == nil {
		return ""
	}
	switch rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
	case *sdcpb.SchemaElem_Leaflist:
	case *sdcpb.SchemaElem_Field:
		return rsp.GetSchema().GetField().GetDefault()
	}
	return ""
}

func toBasicType(sclt *sdcpb.SchemaLeafType, val string) string {
	switch sclt.Type {
	case int64Type, uint64Type, identityrefType, enumType: // TODO: revisit enum type
		return stringType
	case unionType:
		return getTypeUnion(sclt, &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: val}})
	case "leafref":
		// TODO: follow leaf ref and return its type
		return stringType
	default:
		return sclt.Type
	}
}

func getTypeUnion(uType *sdcpb.SchemaLeafType, tv *sdcpb.TypedValue) string {
	for _, ut := range uType.GetUnionTypes() {
		_, err := convertValue(tv.GetStringVal(), ut.GetType())
		if err == nil {
			return ut.GetType()
		}
	}
	// default to string
	return stringType
}

func getLeaflistType(llType *sdcpb.SchemaLeafType, tv *sdcpb.TypedValue) string {
	llTypeStr := ""
	switch llType.GetType() {
	case int64Type, uint64Type, enumType, stringType, identityrefType:
		llTypeStr = stringType
	case leafrefType: // TODO: query leafref schema and return its type
		llTypeStr = stringType
	case unionType:
		llTypeStr = getTypeUnion(llType, tv)
	default:
		llTypeStr = llType.GetType()
	}
	return leafListTypePrefix + llTypeStr
}
