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

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// SchemaClientBound provides access to a certain vendor + model + version based schema
type SchemaClientBound interface {
	// GetSchema retrieves the schema for the given path
	GetSchemaSdcpbPath(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error)
	// GetSchemaElements retrieves the Schema Elements for all levels of the given path
	GetSchemaElements(ctx context.Context, p *sdcpb.Path, done chan struct{}) (chan *sdcpb.GetSchemaResponse, error)
	ToPath(ctx context.Context, path []string) (*sdcpb.Path, error)
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
	upds := make([]*sdcpb.Update, 0)
	rsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, upd.GetPath())
	if err != nil {
		return nil, err
	}

	switch rsp := rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		// log.Debugf("expanding update %v on container %q", upd, rsp.Container.Name)

		if upd.Value == nil {
			rs, err := c.ExpandUpdateKeysAsLeaf(ctx, upd)
			if err != nil {
				return nil, err
			}
			upds := append(upds, rs...)
			return upds, nil
		}

		var v interface{}
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
			switch v := v.(type) {
			case string:
				upd.Value = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
			}
		}

		if rsp.Field.GetType().Type == "identityref" {
			upd.Value, err = Convert(upd.GetValue().GetStringVal(), rsp.Field.GetType())
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

			schemaRsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, intUpd.Path)
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
		// // make sure all keys exist either in the JSON value or
		// // in the path but NOT in both and build keySet
		keySet := map[string]string{}
		for _, k := range cs.Container.GetKeys() {
			if v, ok := jv[k.Name]; ok {
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and JSON value", k.Name)
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
		log.Warnf("unexpected json type cast %T", jv)
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
		arr := strings.SplitN(v, ".", 2)
		digits, err := strconv.ParseInt(arr[0], 10, 64)
		if err != nil {
			return nil, err
		}
		precision64, err := strconv.ParseUint(arr[1], 10, 32)
		if err != nil {
			return nil, err
		}
		precision := uint32(precision64)
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DecimalVal{DecimalVal: &sdcpb.Decimal64{Digits: digits, Precision: precision}},
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

	searchNames := []string{name}
	if i := strings.Index(name, ":"); i >= 0 {
		searchNames = append(searchNames, name[i+1:])
	}

	for _, s := range searchNames {
		if cs.Container.Name == "__root__" {
			for _, c := range cs.Container.GetChildren() {
				rsp, err := scb.GetSchemaSdcpbPath(ctx, &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: c}}})
				if err != nil {
					log.Errorf("Failed to get schema object %s: %v", c, err)
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

func (c *Converter) ConvertTypedValueToProto(ctx context.Context, p *sdcpb.Path, tv *sdcpb.TypedValue) (*sdcpb.TypedValue, error) {
	rsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, p)
	if err != nil {
		return nil, err
	}
	return ConvertTypedValueToYANGType(rsp.GetSchema(), tv)
}

func ConvertTypedValueToYANGType(schemaElem *sdcpb.SchemaElem, tv *sdcpb.TypedValue) (*sdcpb.TypedValue, error) {
	switch {
	case schemaElem.GetContainer() != nil:
		if schemaElem.GetContainer().IsPresence {
			return &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_EmptyVal{},
			}, nil
		}
	case schemaElem.GetLeaflist() != nil:
		switch tv.Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			return tv, nil
		}
		return &sdcpb.TypedValue{
			Timestamp: tv.GetTimestamp(),
			Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{
					Element: []*sdcpb.TypedValue{tv},
				},
			},
		}, nil
	case schemaElem.GetField() != nil:
		switch schemaElem.GetField().GetType().GetType() {
		default:
			return tv, nil
		case "string", "identityref":
			return tv, nil
		case "uint64", "uint32", "uint16", "uint8":
			i, err := strconv.Atoi(TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			ctv := &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_UintVal{UintVal: uint64(i)},
			}
			return ctv, nil
		case "int64", "int32", "int16", "int8":
			i, err := strconv.Atoi(TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			ctv := &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_IntVal{IntVal: int64(i)},
			}
			return ctv, nil
		case "enumeration":
			return tv, nil
		case "union":
			return tv, nil
		case "boolean":
			v, err := strconv.ParseBool(TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: v}}, nil
		case "decimal64":
			d64, err := ParseDecimal64(TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_DecimalVal{
					DecimalVal: d64,
				},
			}, nil
		case "float":
			v, err := strconv.ParseFloat(TypedValueToString(tv), 32)
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_FloatVal{FloatVal: float32(v)},
			}, nil
		}
	}
	return nil, nil
}

func convertUpdateTypedValue(_ context.Context, upd *sdcpb.Update, scRsp *sdcpb.GetSchemaResponse, leaflists map[string]*leafListNotification) (*sdcpb.Update, error) {
	switch {
	case scRsp.GetSchema().GetContainer() != nil:
		if !scRsp.GetSchema().GetContainer().GetIsPresence() {
			return nil, nil
		}
		return upd, nil
	case scRsp.GetSchema().GetLeaflist() != nil:
		// leaf-list received as a key
		if upd.GetValue() == nil {
			// clone path
			p := proto.Clone(upd.GetPath()).(*sdcpb.Path)
			// grab the key from the last elem, that's the leaf-list value
			var lftv *sdcpb.TypedValue
			for _, v := range p.GetElem()[len(p.GetElem())-1].GetKey() {
				lftv = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
				break
			}
			if lftv == nil {
				return nil, fmt.Errorf("malformed leaf-list update: %v", upd)
			}
			// delete the key from the last elem (that's the leaf-list value)
			p.GetElem()[len(p.GetElem())-1].Key = nil
			// build unique path
			sp := ToXPath(p, false)
			if _, ok := leaflists[sp]; !ok {
				leaflists[sp] = &leafListNotification{
					path:      p,                               // modified path
					leaflists: make([]*sdcpb.TypedValue, 0, 1), // at least one elem
				}
			}
			// convert leaf-list to its YANG type
			clftv, err := TypedValueToYANGType(lftv, scRsp.GetSchema())
			if err != nil {
				return nil, err
			}
			// append leaf-list
			leaflists[sp].leaflists = append(leaflists[sp].leaflists, clftv)
			return nil, nil
		}
		// regular leaf list
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			return upd, nil
		default:
			return nil, fmt.Errorf("unexpected leaf-list typedValue: %v", upd.GetValue())
		}
	case scRsp.GetSchema().GetField() != nil:
		ctv, err := TypedValueToYANGType(upd.GetValue(), scRsp.GetSchema())
		if err != nil {
			return nil, err
		}
		return &sdcpb.Update{
			Path:  upd.GetPath(),
			Value: ctv,
		}, nil
	}
	return nil, nil
}

// conversion
type leafListNotification struct {
	path      *sdcpb.Path
	leaflists []*sdcpb.TypedValue
}

func (c *Converter) ConvertNotificationTypedValues(ctx context.Context, n *sdcpb.Notification) (*sdcpb.Notification, error) {
	// this map serves as a context to group leaf-lists
	// sent as keys in separate updates.
	leaflists := map[string]*leafListNotification{}
	nn := &sdcpb.Notification{
		Timestamp: n.GetTimestamp(),
		Update:    make([]*sdcpb.Update, 0, len(n.GetUpdate())),
		Delete:    n.GetDelete(),
	}
	// convert typed values to their YANG type
	for _, upd := range n.GetUpdate() {
		StripPathElemPrefixPath(upd.GetPath())
		scRsp, err := c.schemaClientBound.GetSchemaSdcpbPath(ctx, upd.GetPath())
		if err != nil {
			return nil, err
		}
		nup, err := convertUpdateTypedValue(ctx, upd, scRsp, leaflists)
		if err != nil {
			return nil, err
		}
		log.Debugf("converted update from: %v, to: %v", upd, nup)
		// gNMI get() could return a Notification with a single path element containing a JSON/JSON_IETF blob, we need to expand this into several typed values.
		if nup == nil && (upd.GetValue().GetJsonVal() != nil || upd.GetValue().GetJsonIetfVal() != nil) {
			expUpds, err := c.ExpandUpdate(ctx, upd)
			if err != nil {
				return nil, err
			}
			expNn := &sdcpb.Notification{
				Timestamp: n.GetTimestamp(),
				Update:    expUpds,
				Delete:    n.GetDelete(),
			}
			return c.ConvertNotificationTypedValues(ctx, expNn)
		}
		if nup == nil { // filters out notification ending in non-presence containers
			continue
		}
		nn.Update = append(nn.Update, nup)
	}
	// add accumulated leaf-lists
	for _, lfnotif := range leaflists {
		nn.Update = append(nn.Update, &sdcpb.Update{
			Path: lfnotif.path,
			Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{Element: lfnotif.leaflists},
			},
			},
		})
	}

	return nn, nil
}
