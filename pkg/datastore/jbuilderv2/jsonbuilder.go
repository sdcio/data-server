package jbuilderv2

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	schemaClient "github.com/iptecharch/data-server/pkg/datastore/clients/schema"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

type jsonBuilder struct {
	scc *schemaClient.SchemaClientBound
}

type pathElem struct {
	name    string
	key     map[string]string
	keyType map[string]string
}

func (pe *pathElem) String() string {
	s := pe.name
	keys := make([]string, 0, len(pe.key))
	for k := range pe.key {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s += fmt.Sprintf("[%s=%s:%s]", k, pe.key[k], pe.keyType[k])
	}
	return s
}

type update struct {
	path      []*pathElem
	value     string
	valueType string
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
// If the path points to a field (i.e leaf), the typedValue should not be nil and is expected to be a StringVal.
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

	u := update{valueType: vt}
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

	convertedValue, err := convertValue(u.value, u.valueType)
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
	switch valueType {
	case "string":
		return value, nil
	case "int8", "int16", "int32", "uint8", "uint16", "uint32":
		return strconv.Atoi(value)
	case "float":
		return strconv.ParseFloat(value, 64)
	case "boolean":
		return strconv.ParseBool(value)
	case "null":
		return nil, nil
	case "PRESENCE":
		return map[string]any{}, nil
	case "LEAFLIST":
		return []any{value}, nil
	case "decimal64":
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
				case []any: // leaf-list
					obj[elem.name] = append(obj[elem.name].([]any), value.([]any)...)
				default: // leaf, presence container
					obj[elem.name] = value
				}
			default:
				obj[elem.name] = value
			}
			continue
		}
		// Handle nested objects or arrays
		if len(elem.key) == 0 {
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
		// Handle keys for arrays of objects
		if _, ok := obj[elem.name]; !ok {
			obj[elem.name] = make([]map[string]interface{}, 0)
		}

		arr, ok := obj[elem.name].([]map[string]interface{})
		if !ok {
			return fmt.Errorf("failed: path element '%s' is not an array of objects as expected: %T", elem.name, obj[elem.name])
		}

		matchedObj, found, err := findOrCreateMatchingObject(arr, elem.key, elem.keyType)
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
func findOrCreateMatchingObject(arr []map[string]interface{}, keys, keyTypes map[string]string) (map[string]interface{}, bool, error) {
	for _, obj := range arr {
		match := true
		for k, v := range keys {
			convertedValue, err := convertValue(v, keyTypes[k])
			if err != nil || !reflect.DeepEqual(obj[k], convertedValue) {
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
		convertedValue, err := convertValue(v, keyTypes[k])
		if err != nil {
			return nil, false, fmt.Errorf("failed converting key value: %v:%s: %v", v, keyTypes[k], err)
		}
		newObj[k] = convertedValue
	}
	return newObj, false, nil
}

func (j *jsonBuilder) getType(ctx context.Context, tv *sdcpb.TypedValue, rsp *sdcpb.GetSchemaResponse) (string, bool, error) {
	switch rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		return "PRESENCE", getDefault(rsp) == tv.GetStringVal(), nil
	case *sdcpb.SchemaElem_Leaflist:
		return "LEAFLIST", getDefault(rsp) == tv.GetStringVal(), nil
	case *sdcpb.SchemaElem_Field:
		// fmt.Println("!!GOT FIELD", p, tv)
		switch t := rsp.Schema.GetField().GetType().GetType(); t {
		case "int64", "uint64", "enumeration":
			return "string", getDefault(rsp) == tv.GetStringVal(), nil
		case "union":
			// TODO: go through union types and compare the given value to the type
			for _, ut := range rsp.Schema.GetField().GetType().GetUnionTypes() {
				_, err := convertValue(tv.GetStringVal(), ut.GetType())
				if err == nil {
					return ut.GetType(), rsp.Schema.GetField().GetDefault() == tv.GetStringVal(), nil
				}
			}
			return "string", getDefault(rsp) == tv.GetStringVal(), nil
		case "identityref":
			return "string", getDefault(rsp) == tv.GetStringVal(), nil
		case "leafref":
			// TODO: query leafref schema and return its type
			return "string", getDefault(rsp) == tv.GetStringVal(), nil
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
			name:    sdcpe.GetName(),
			key:     sdcpe.GetKey(),
			keyType: map[string]string{},
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
				pe.keyType[keySchema.GetName()] = toBasicType(keySchema.GetType(), pe.key[keySchema.GetName()])
			}
		case baseListSchema.GetSchema().GetLeaflist() != nil:
			pe.keyType[sdcpe.GetName()] = toBasicType(baseListSchema.GetSchema().GetLeaflist().GetType(), pe.key[sdcpe.GetName()])
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
	case "int64", "uint64", "enumeration":
		return "string"
	case "union":
		// TODO: go through union types and compare the given value to the type
		for _, ut := range sclt.GetUnionTypes() {
			_, err := convertValue(val, ut.GetType())
			if err == nil {
				return ut.GetType()
			}
		}
		// default to string
		return "string"
	case "identityref":
		return "string"
	case "leafref":
		// TODO: follow leaf ref and return its type
		return "string"
	default:
		return sclt.Type
	}
}
