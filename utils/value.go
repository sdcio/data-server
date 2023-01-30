package utils

import (
	"encoding/json"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
)

func GetValue(updValue *gnmi.TypedValue) (interface{}, error) {
	if updValue == nil {
		return nil, nil
	}
	var value interface{}
	var jsondata []byte
	switch updValue.Value.(type) {
	case *gnmi.TypedValue_AsciiVal:
		value = updValue.GetAsciiVal()
	case *gnmi.TypedValue_BoolVal:
		value = updValue.GetBoolVal()
	case *gnmi.TypedValue_BytesVal:
		value = updValue.GetBytesVal()
	case *gnmi.TypedValue_DecimalVal:
		//lint:ignore SA1019 still need DecimalVal for backward compatibility
		value = updValue.GetDecimalVal()
	case *gnmi.TypedValue_FloatVal:
		//lint:ignore SA1019 still need GetFloatVal for backward compatibility
		value = updValue.GetFloatVal()
	case *gnmi.TypedValue_DoubleVal:
		value = updValue.GetDoubleVal()
	case *gnmi.TypedValue_IntVal:
		value = updValue.GetIntVal()
	case *gnmi.TypedValue_StringVal:
		value = updValue.GetStringVal()
	case *gnmi.TypedValue_UintVal:
		value = updValue.GetUintVal()
	case *gnmi.TypedValue_JsonIetfVal:
		jsondata = updValue.GetJsonIetfVal()
	case *gnmi.TypedValue_JsonVal:
		jsondata = updValue.GetJsonVal()
	case *gnmi.TypedValue_LeaflistVal:
		value = updValue.GetLeaflistVal()
	case *gnmi.TypedValue_ProtoBytes:
		value = updValue.GetProtoBytes()
	case *gnmi.TypedValue_AnyVal:
		value = updValue.GetAnyVal()
	}
	if value == nil && len(jsondata) != 0 {
		err := json.Unmarshal(jsondata, &value)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func GetSchemaValue(updValue *schemapb.TypedValue) (interface{}, error) {
	if updValue == nil {
		return nil, nil
	}
	var value interface{}
	var jsondata []byte
	switch updValue.Value.(type) {
	case *schemapb.TypedValue_AsciiVal:
		value = updValue.GetAsciiVal()
	case *schemapb.TypedValue_BoolVal:
		value = updValue.GetBoolVal()
	case *schemapb.TypedValue_BytesVal:
		value = updValue.GetBytesVal()
	case *schemapb.TypedValue_DecimalVal:
		value = updValue.GetDecimalVal()
	case *schemapb.TypedValue_FloatVal:
		value = updValue.GetFloatVal()
	// case *schemapb.TypedValue_DoubleVal:
	// 	value = updValue.GetDoubleVal()
	case *schemapb.TypedValue_IntVal:
		value = updValue.GetIntVal()
	case *schemapb.TypedValue_StringVal:
		value = updValue.GetStringVal()
	case *schemapb.TypedValue_UintVal:
		value = updValue.GetUintVal()
	case *schemapb.TypedValue_JsonIetfVal:
		jsondata = updValue.GetJsonIetfVal()
	case *schemapb.TypedValue_JsonVal:
		jsondata = updValue.GetJsonVal()
	case *schemapb.TypedValue_LeaflistVal:
		value = updValue.GetLeaflistVal()
	case *schemapb.TypedValue_ProtoBytes:
		value = updValue.GetProtoBytes()
	case *schemapb.TypedValue_AnyVal:
		value = updValue.GetAnyVal()
	}
	if value == nil && len(jsondata) != 0 {
		err := json.Unmarshal(jsondata, &value)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func ToSchemaTypedValue(v any) *schemapb.TypedValue {
	log.Debugf("to schema value %T, %#v", v, v) //TODO2: Writing in schemapb typedValue, need to change to gNMI ?
	switch v := v.(type) {
	case *schemapb.TypedValue:
		return v
	case *gnmi.TypedValue:
		return ToSchemaTypedValue(v.GetValue())
	case *gnmi.TypedValue_AnyVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_AnyVal{
				AnyVal: v.AnyVal,
			},
		}
	case *gnmi.TypedValue_AsciiVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_AsciiVal{
				AsciiVal: v.AsciiVal,
			},
		}
	case *gnmi.TypedValue_StringVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_StringVal{
				StringVal: v.StringVal,
			},
		}
	case *gnmi.TypedValue_BoolVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_BoolVal{
				BoolVal: v.BoolVal,
			},
		}
	case *gnmi.TypedValue_BytesVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_BytesVal{
				BytesVal: v.BytesVal,
			},
		}
	case *gnmi.TypedValue_IntVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_IntVal{
				IntVal: v.IntVal,
			},
		}
	case *gnmi.TypedValue_UintVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_UintVal{
				UintVal: v.UintVal,
			},
		}
	case *gnmi.TypedValue_ProtoBytes:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_ProtoBytes{
				ProtoBytes: v.ProtoBytes,
			},
		}
	case *gnmi.TypedValue_LeaflistVal:
		schemalf := &schemapb.ScalarArray{
			Element: make([]*schemapb.TypedValue, 0, len(v.LeaflistVal.GetElement())),
		}
		for _, e := range v.LeaflistVal.GetElement() {
			schemalf.Element = append(schemalf.Element, ToSchemaTypedValue(e))
		}
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_LeaflistVal{LeaflistVal: schemalf},
		}
	}
	return nil
}

// func ToGNMITypedValue(v any) *gnmi.TypedValue {
// 	log.Infof("to gNMI value %T, %v", v, v)
// 	switch v := v.(type) {
// 	case *schemapb.TypedValue_AsciiVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AsciiVal{
// 				AsciiVal: v.AsciiVal,
// 			},
// 		}
// 	case *schemapb.TypedValue_StringVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_StringVal{
// 				StringVal: v.StringVal,
// 			},
// 		}
// 	case string:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AsciiVal{
// 				AsciiVal: v,
// 			},
// 		}
// 	}
// 	return nil
// }

func ToGNMITypedValue(v *schemapb.TypedValue) *gnmi.TypedValue {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *schemapb.TypedValue_AnyVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
		}
	case *schemapb.TypedValue_AsciiVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
		}
	case *schemapb.TypedValue_BoolVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
		}
	case *schemapb.TypedValue_BytesVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
		}
	// case *schemapb.TypedValue_DecimalVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
	// 	}
	// case *schemapb.TypedValue_FloatVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
	// 	}
	case *schemapb.TypedValue_IntVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_IntVal{IntVal: v.GetIntVal()},
		}
	case *schemapb.TypedValue_JsonIetfVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
		}
	case *schemapb.TypedValue_JsonVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
		}
	case *schemapb.TypedValue_LeaflistVal:
		gnmilf := &gnmi.ScalarArray{
			Element: make([]*gnmi.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
		}
		for _, e := range v.GetLeaflistVal().GetElement() {
			gnmilf.Element = append(gnmilf.Element, ToGNMITypedValue(e))
		}
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_LeaflistVal{LeaflistVal: gnmilf},
		}
	case *schemapb.TypedValue_ProtoBytes:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
		}
	case *schemapb.TypedValue_StringVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetStringVal()},
		}
	case *schemapb.TypedValue_UintVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_UintVal{UintVal: v.GetUintVal()},
		}
	}
	return nil
}
