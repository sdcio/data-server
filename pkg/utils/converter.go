package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/sdcio/logger"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

// SchemaClientBound provides access to a certain vendor + model + version based schema
type SchemaClientBound interface {
	// GetSchema retrieves the schema for the given path
	GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error)
	// GetSchemaElements retrieves the Schema Elements for all levels of the given path
	GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error)
}

type Converter struct {
	schemaClientBound SchemaClientBound
}

func NewConverter(scb SchemaClientBound) *Converter {
	return &Converter{
		schemaClientBound: scb,
	}
}

func (c *Converter) ExpandUpdates(ctx context.Context, updates []*sdcpb.Update) ([]*sdcpb.Update, error) {
	outUpdates := make([]*sdcpb.Update, 0, len(updates))
	for idx, upd := range updates {
		_ = idx
		expUpds, err := c.ExpandUpdate(ctx, upd)
		if err != nil {
			return nil, err
		}
		outUpdates = append(outUpdates, expUpds...)
	}
	return outUpdates, nil
}

// expandUpdate Expands the value, in case of json to single typed value updates
func (c *Converter) ExpandUpdate(ctx context.Context, upd *sdcpb.Update) ([]*sdcpb.Update, error) {
	log := logf.FromContext(ctx)
	upds := make([]*sdcpb.Update, 0)
	rsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, upd.GetPath())
	if err != nil {
		return nil, err
	}

	p := upd.GetPath()
	_ = p

	// skip state
	if rsp.GetSchema().IsState() {
		return nil, nil
	}

	switch rsp := rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		if upd.Value == nil {
			// if it is a presence container and no value is set, set upd value to EmptyVal
			if rsp.Container.GetIsPresence() {
				upd.Value = &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_EmptyVal{},
				}
				return append(upds, upd), nil
			}
			if len(upd.Path.Elem) > 0 && len(upd.Path.Elem[len(upd.Path.Elem)-1].Key) > 0 {
				newUpd := &sdcpb.Update{}
				// if value is nil but the last path elem contains keys
				for k, v := range upd.Path.Elem[len(upd.Path.Elem)-1].Key {

					// deepcopy via marshall unmarshall
					newUpd = proto.Clone(upd).(*sdcpb.Update)

					// adjust path
					newUpd.Path.Elem = append(newUpd.Path.Elem, &sdcpb.PathElem{Name: k})
					rsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, newUpd.GetPath())
					if err != nil {
						return nil, err
					}
					// convert key string value to real typedvalue
					newUpd.Value, err = ConvertToTypedValue(rsp.GetSchema(), v, 0)
					if err != nil {
						return nil, err
					}
				}
				upds = append(upds, newUpd)
				return upds, nil
			}
			return nil, nil
		}
		var v any
		var err error
		var jsonDecoder *json.Decoder
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_JsonIetfVal:
			jsonDecoder = json.NewDecoder(bytes.NewReader(upd.GetValue().GetJsonIetfVal()))
		case *sdcpb.TypedValue_JsonVal:
			jsonDecoder = json.NewDecoder(bytes.NewReader(upd.GetValue().GetJsonVal()))
		default:
			return []*sdcpb.Update{upd}, nil
		}
		// don't decode into float64 but keep as a string
		// this solves issues created by reading long integers
		jsonDecoder.UseNumber()
		err = jsonDecoder.Decode(&v)
		if err != nil {
			return nil, err
		}
		// log.Debugf("update has jsonVal: %T, %v\n", v, v)
		rs, err := c.ExpandContainerValue(ctx, upd.GetPath(), v, rsp)
		if err != nil {
			return nil, err
		}
		upds := append(upds, rs...)
		return upds, nil

	case *sdcpb.SchemaElem_Field:
		var v interface{}
		var err error

		var jsonValue []byte
		if upd.GetValue() == nil {
			log.V(logger.VError).Info("Value is nil", "Path", upd.Path.ToXPath(false))
			return nil, nil
		}
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_JsonVal:
			jsonValue = upd.GetValue().GetJsonVal()
		case *sdcpb.TypedValue_JsonIetfVal:
			jsonValue = upd.GetValue().GetJsonIetfVal()
		}

		// process value
		if jsonValue != nil {
			err = json.Unmarshal(jsonValue, &v)
			if err != nil {
				return nil, err
			}
			upd.Value = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: string(jsonValue)}}
		}

		if upd.Value.GetStringVal() != "" && rsp.Field.GetType().GetTypeName() != "string" {
			upd.Value, err = Convert(ctx, upd.GetValue().GetStringVal(), rsp.Field.GetType())
			if err != nil {
				return nil, err
			}
		}

		// We expect that all identityrefs are sent by schema-server as a identityref type now, not string
		if rsp.Field.GetType().Type == "identityref" && upd.GetValue().GetStringVal() != "" {
			upd.Value, err = Convert(ctx, upd.GetValue().GetStringVal(), rsp.Field.GetType())
			if err != nil {
				return nil, err
			}
		}

		upds = append(upds, upd)
		return upds, nil
	case *sdcpb.SchemaElem_Leaflist:
		upds = append(upds, upd)
		return upds, nil
	}
	return nil, nil
}

func (c *Converter) ExpandUpdateKeysAsLeaf(ctx context.Context, upd *sdcpb.Update) ([]*sdcpb.Update, error) {
	upds := make([]*sdcpb.Update, 0)
	var err error
	var schemaRsp *sdcpb.GetSchemaResponse
	// expand update path if it contains keys
	for i, pe := range upd.GetPath().GetElem() {
		if len(pe.GetKey()) == 0 {
			continue
		}

		for k, v := range pe.GetKey() {
			intUpd := &sdcpb.Update{
				Path: &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, 0, i+1+1),
				},
			}
			for j := 0; j <= i; j++ {
				intUpd.Path.Elem = append(intUpd.Path.Elem,
					&sdcpb.PathElem{
						Name: upd.GetPath().GetElem()[j].GetName(),
						Key:  upd.GetPath().GetElem()[j].GetKey(),
					},
				)
			}
			intUpd.Path.Elem = append(intUpd.Path.Elem, &sdcpb.PathElem{Name: k})

			schemaRsp, err = c.schemaClientBound.GetSchemaSdcpbPath(ctx, intUpd.Path)
			if err != nil {
				return nil, err
			}

			intUpd.Value, err = TypedValueToYANGType(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}, schemaRsp.GetSchema())
			if err != nil {
				return nil, err
			}

			upds = append(upds, intUpd)
		}
	}

	return upds, nil
}

func (c *Converter) ExpandContainerValue(ctx context.Context, p *sdcpb.Path, jv any, cs *sdcpb.SchemaElem_Container) ([]*sdcpb.Update, error) {
	log := logf.FromContext(ctx)
	// log.Debugf("expanding jsonVal %T | %v | %v", jv, jv, p)
	switch jv := jv.(type) {
	case string:
		v := strings.Trim(jv, "\"")
		return []*sdcpb.Update{
			{
				Path: p,
				Value: &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_StringVal{StringVal: v},
				},
			},
		}, nil
	case map[string]any:
		upds := make([]*sdcpb.Update, 0)
		// make sure all keys are present
		// and append them to path
		var keysInPath map[string]string
		if numElems := len(p.GetElem()); numElems > 0 {
			keysInPath = p.GetElem()[numElems-1].GetKey()
		}
		// make sure all keys exist either in the JSON value or
		// in the path but NOT in both and build keySet
		keySet := map[string]string{}
		for _, k := range cs.Container.GetKeys() {
			if v, ok := jv[k.Name]; ok {
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and JSON value", k.Name)
				}
				// in case of json_ietf we need to cut the module prefix
				if k.Type.Type == "identityref" {
					val := v
					found := false
					_, val, found = strings.Cut(fmt.Sprintf("%v", v), ":")
					if found {
						v = val
					}
				}
				keySet[k.Name] = fmt.Sprintf("%v", v)
				continue
			}
			if v, ok := keysInPath[k.Name]; ok {
				keySet[k.Name] = v
				continue
			}
			return nil, fmt.Errorf("missing key %s in element %s", k.Name, cs.Container.GetName())
		}
		// handling keys in last element of the path or in the json value
		for _, k := range cs.Container.GetKeys() {
			if _, ok := jv[k.Name]; ok {
				// log.Debugf("handling key %s", k.Name)
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and JSON value", k.Name)
				}
				if p.GetElem()[len(p.GetElem())-1].Key == nil {
					p.GetElem()[len(p.GetElem())-1].Key = make(map[string]string)
				}
				p.GetElem()[len(p.GetElem())-1].Key = keySet
				continue
			}
			// if key is not in the value it must be set in the path
			if _, ok := keysInPath[k.Name]; !ok {
				return nil, fmt.Errorf("missing key %q from list %q", k.Name, cs.Container.Name)
			}
		}
		for k, v := range jv {
			// TODO remove the statement_annotate again ...
			if k == "_annotate" {
				continue
			}
			item, ok := getItem(ctx, k, cs, c.schemaClientBound)
			if !ok {
				return nil, fmt.Errorf("unknown object %q under container %q", k, cs.Container.Name)
			}
			switch item := item.(type) {
			case *sdcpb.LeafSchema: // field
				// log.Debugf("handling field %s", item.Name)
				np := proto.Clone(p).(*sdcpb.Path)
				np.Elem = append(np.Elem, &sdcpb.PathElem{Name: item.Name})
				upd := &sdcpb.Update{Path: np}
				switch item.GetType().GetType() {
				case "empty":
					upd.Value = &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_EmptyVal{},
					}
				default:
					schemaRsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, np)
					if err != nil {
						return nil, err
					}
					upd.Value, err = TypedValueToYANGType(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: fmt.Sprintf("%v", v)}}, schemaRsp.GetSchema())
					if err != nil {
						return nil, err
					}

				}
				upds = append(upds, upd)
			case *sdcpb.LeafListSchema: // leaflist
				// log.Debugf("TODO: handling leafList %s", item.Name)
				np := proto.Clone(p).(*sdcpb.Path)
				np.Elem = append(np.Elem, &sdcpb.PathElem{Name: item.Name})

				se := &sdcpb.SchemaElem{
					Schema: &sdcpb.SchemaElem_Leaflist{
						Leaflist: item,
					},
				}

				list := []*sdcpb.TypedValue{}
				// iterate through the elements
				// ATTENTION: assuming its all strings
				// add them to the Leaflist value
				switch x := v.(type) {
				case []any:
					for _, e := range x {
						tv := &sdcpb.TypedValue{
							Value: &sdcpb.TypedValue_StringVal{
								StringVal: fmt.Sprintf("%v", e),
							},
						}
						tvYangType, err := TypedValueToYANGType(tv, se)
						if err != nil {
							return nil, err
						}
						list = append(list, tvYangType)
					}
				default:
					return nil, fmt.Errorf("leaflist %s expects array as input, but %v of type %v was given", np.String(), x, reflect.TypeOf(x).Name())
				}

				upd := &sdcpb.Update{
					Path: np,
					Value: &sdcpb.TypedValue{
						Timestamp: 0,
						Value:     &sdcpb.TypedValue_LeaflistVal{LeaflistVal: &sdcpb.ScalarArray{Element: list}},
					},
				}
				upds = append(upds, upd)

			case string: // child container
				// log.Debugf("handling child container %s", item)
				np := proto.Clone(p).(*sdcpb.Path)
				np.Elem = append(np.Elem, &sdcpb.PathElem{Name: item})
				rsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, np)
				if err != nil {
					return nil, err
				}
				switch rsp := rsp.GetSchema().Schema.(type) {
				case *sdcpb.SchemaElem_Container:
					var rs []*sdcpb.Update
					// code for presence containers
					m, ok := v.(map[string]any)
					if ok && len(m) == 0 && rsp.Container.IsPresence {
						rs = []*sdcpb.Update{
							{
								Path: np,
								Value: &sdcpb.TypedValue{
									Value: &sdcpb.TypedValue_EmptyVal{},
								},
							}}
					} else {
						rs, err = c.ExpandContainerValue(ctx, np, v, rsp)
						if err != nil {
							return nil, err
						}
					}

					upds = append(upds, rs...)
				default:
					// should not happen
					return nil, fmt.Errorf("object %q is not a container", item)
				}
			default:
				return nil, fmt.Errorf("unknown object %q under container %q", k, cs.Container.Name)
			}
		}
		return upds, nil
	case []any:
		upds := make([]*sdcpb.Update, 0)
		for _, v := range jv {
			np := proto.Clone(p).(*sdcpb.Path)
			r, err := c.ExpandContainerValue(ctx, np, v, cs)
			if err != nil {
				return nil, err
			}
			upds = append(upds, r...)
		}
		return upds, nil
	default:
		log.Error(nil, "unexpected json type cast", "type", reflect.TypeOf(jv).String())
		return nil, nil
	}
}

func TypedValueToYANGType(tv *sdcpb.TypedValue, schemaObject *sdcpb.SchemaElem) (*sdcpb.TypedValue, error) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_AsciiVal:
		return ConvertToTypedValue(schemaObject, tv.GetAsciiVal(), tv.GetTimestamp())
	case *sdcpb.TypedValue_BoolVal:
		return tv, nil
	case *sdcpb.TypedValue_BytesVal:
		return tv, nil
	case *sdcpb.TypedValue_DecimalVal:
		return tv, nil
	case *sdcpb.TypedValue_FloatVal:
		return tv, nil
	case *sdcpb.TypedValue_DoubleVal:
		return tv, nil
	case *sdcpb.TypedValue_IntVal:
		return tv, nil
	case *sdcpb.TypedValue_StringVal:
		return ConvertToTypedValue(schemaObject, tv.GetStringVal(), tv.GetTimestamp())
	case *sdcpb.TypedValue_UintVal:
		return tv, nil
	case *sdcpb.TypedValue_JsonIetfVal: // TODO:
	case *sdcpb.TypedValue_JsonVal: // TODO:
	case *sdcpb.TypedValue_LeaflistVal:
		return tv, nil
	case *sdcpb.TypedValue_ProtoBytes:
		return tv, nil
	case *sdcpb.TypedValue_AnyVal:
		return tv, nil
	case *sdcpb.TypedValue_IdentityrefVal:
		return ConvertToTypedValue(schemaObject, tv.GetStringVal(), tv.GetTimestamp())
	}
	return tv, nil
}

func ConvertToTypedValue(schemaObject *sdcpb.SchemaElem, v string, ts uint64) (*sdcpb.TypedValue, error) {
	var schemaType *sdcpb.SchemaLeafType
	switch {
	case schemaObject.GetField() != nil:
		schemaType = schemaObject.GetField().GetType()
	case schemaObject.GetLeaflist() != nil:
		schemaType = schemaObject.GetLeaflist().GetType()
	case schemaObject.GetContainer() != nil:
		if !schemaObject.GetContainer().IsPresence {
			return nil, errors.New("non presence container update")
		}
		return nil, nil
	}
	return convertStringToTv(schemaType, v, ts)
}

func convertStringToTv(schemaType *sdcpb.SchemaLeafType, v string, ts uint64) (*sdcpb.TypedValue, error) {
	// convert field or leaf-list schema elem
	switch schemaType.GetType() {
	case "string":
		return &sdcpb.TypedValue{

			Value: &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "uint64", "uint32", "uint16", "uint8":
		i, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_UintVal{UintVal: i},
		}, nil
	case "int64", "int32", "int16", "int8":
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_IntVal{IntVal: i},
		}, nil
	case "boolean":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_BoolVal{BoolVal: b},
		}, nil
	case "decimal64":
		decimalVal, err := ParseDecimal64(v)
		if err != nil {
			return nil, err
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DecimalVal{
				DecimalVal: decimalVal,
			},
		}, nil
	case "identityref":
		before, name, found := strings.Cut(v, ":")
		if !found {
			name = before
		}
		prefix, ok := schemaType.IdentityPrefixesMap[name]
		if !ok {
			identities := make([]string, 0, len(schemaType.IdentityPrefixesMap))
			for k := range schemaType.IdentityPrefixesMap {
				identities = append(identities, k)
			}
			return nil, fmt.Errorf("identity %s not found, possible values are %s", v, strings.Join(identities, ", "))
		}
		module, ok := schemaType.ModulePrefixMap[name]
		if !ok {
			identities := make([]string, 0, len(schemaType.IdentityPrefixesMap))
			for k := range schemaType.IdentityPrefixesMap {
				identities = append(identities, k)
			}
			return nil, fmt.Errorf("identity %s not found, possible values are %s", v, strings.Join(identities, ", "))
		}
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_IdentityrefVal{IdentityrefVal: &sdcpb.IdentityRef{Value: name, Prefix: prefix, Module: module}},
		}, nil
	case "leafref":
		return convertStringToTv(schemaType.LeafrefTargetType, v, ts)
	case "union":
		for _, ut := range schemaType.GetUnionTypes() {
			tv, err := convertStringToTv(ut, v, ts)
			if err == nil {
				return tv, nil
			}
		}
		return nil, fmt.Errorf("invalid value %s for union type: %v", v, schemaType)
	case "enumeration":
		// TODO: get correct type, assuming string
		return &sdcpb.TypedValue{
			Timestamp: ts,
			Value:     &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "": // presence ?
		return &sdcpb.TypedValue{}, nil
	}
	return nil, nil
}

func getItem(ctx context.Context, s string, cs *sdcpb.SchemaElem_Container, scb SchemaClientBound) (any, bool) {
	f, ok := getField(s, cs)
	if ok {
		return f, true
	}
	lfl, ok := getLeafList(s, cs)
	if ok {
		return lfl, true
	}
	c, ok := getChild(ctx, s, cs, scb)
	if ok {
		return c, true
	}
	k, ok := getKey(s, cs)
	if ok {
		return k, true
	}
	return nil, false
}

func getKey(s string, cs *sdcpb.SchemaElem_Container) (*sdcpb.LeafSchema, bool) {
	for _, f := range cs.Container.GetKeys() {
		if f.Name == s {
			return f, true
		}
		if fmt.Sprintf("%s:%s", f.ModuleName, f.Name) == s {
			return f, true
		}
	}
	return nil, false
}

func getField(s string, cs *sdcpb.SchemaElem_Container) (*sdcpb.LeafSchema, bool) {
	for _, f := range cs.Container.GetFields() {
		if f.Name == s {
			return f, true
		}
		if fmt.Sprintf("%s:%s", f.ModuleName, f.Name) == s {
			return f, true
		}
	}
	return nil, false
}

func getLeafList(s string, cs *sdcpb.SchemaElem_Container) (*sdcpb.LeafListSchema, bool) {
	for _, lfl := range cs.Container.GetLeaflists() {
		if lfl.Name == s {
			return lfl, true
		}
		if fmt.Sprintf("%s:%s", lfl.ModuleName, lfl.Name) == s {
			return lfl, true
		}
	}
	return nil, false
}

func getChild(ctx context.Context, name string, cs *sdcpb.SchemaElem_Container, scb SchemaClientBound) (any, bool) {
	log := logf.FromContext(ctx)

	searchNames := []string{name}
	if i := strings.Index(name, ":"); i >= 0 {
		searchNames = append(searchNames, name[i+1:])
	}

	for _, s := range searchNames {
		if cs.Container.Name == "__root__" {
			for _, c := range cs.Container.GetChildren() {
				rsp, err := scb.GetSchemaSdcpbPath(ctx, &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: c}}})
				if err != nil {
					log.Error(err, "failed to get schema object", "schema-object", c)
					return "", false
				}
				switch rsp := rsp.GetSchema().Schema.(type) {
				case *sdcpb.SchemaElem_Container:
					for _, child := range rsp.Container.GetChildren() {
						if child == s {
							return child, true
						}
					}
					for _, field := range rsp.Container.GetFields() {
						if field.Name == s {
							return field, true
						}
					}
				default:
					continue
				}
			}
		}
		for _, c := range cs.Container.GetChildren() {
			if c == s {
				return c, true
			}
		}
	}
	return "", false
}
