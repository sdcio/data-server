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

package utils

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/schema-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
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

func GetJsonValue(tv *sdcpb.TypedValue, ietf bool) (any, error) {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_EmptyVal:
		return map[string]any{}, nil
	case *sdcpb.TypedValue_LeaflistVal:
		rs := make([]any, 0, len(tv.GetLeaflistVal().GetElement()))
		for _, e := range tv.GetLeaflistVal().GetElement() {
			val, err := GetJsonValue(e, ietf)
			if err != nil {
				return nil, err
			}
			rs = append(rs, val)
		}
		return rs, nil
	case *sdcpb.TypedValue_IdentityrefVal:
		if ietf {
			return tv.GetIdentityrefVal().JsonIetfString(), nil
		}
		return GetSchemaValue(tv)
	case *sdcpb.TypedValue_DecimalVal:
		// TODO have a String() function on the *sdcpb.TypedValue_DecimalVal type?
		return tv.ToString(), nil
	default:
		return GetSchemaValue(tv)
	}
}

func GetSchemaValue(updValue *sdcpb.TypedValue) (interface{}, error) {
	if updValue == nil {
		return nil, nil
	}
	var value interface{}
	var jsondata []byte
	switch updValue.Value.(type) {
	case *sdcpb.TypedValue_AsciiVal:
		value = updValue.GetAsciiVal()
	case *sdcpb.TypedValue_BoolVal:
		value = updValue.GetBoolVal()
	case *sdcpb.TypedValue_BytesVal:
		value = updValue.GetBytesVal()
	case *sdcpb.TypedValue_DecimalVal:
		value = updValue.GetDecimalVal()
	case *sdcpb.TypedValue_EmptyVal:
		value = updValue.GetEmptyVal()
	case *sdcpb.TypedValue_FloatVal:
		value = updValue.GetFloatVal()
	case *sdcpb.TypedValue_DoubleVal:
		value = updValue.GetDoubleVal()
	case *sdcpb.TypedValue_IntVal:
		value = updValue.GetIntVal()
	case *sdcpb.TypedValue_StringVal:
		value = updValue.GetStringVal()
	case *sdcpb.TypedValue_UintVal:
		value = updValue.GetUintVal()
	case *sdcpb.TypedValue_JsonIetfVal:
		jsondata = updValue.GetJsonIetfVal()
	case *sdcpb.TypedValue_JsonVal:
		jsondata = updValue.GetJsonVal()
	case *sdcpb.TypedValue_LeaflistVal:
		value = updValue.GetLeaflistVal()
	case *sdcpb.TypedValue_ProtoBytes:
		value = updValue.GetProtoBytes()
	case *sdcpb.TypedValue_AnyVal:
		value = updValue.GetAnyVal()
	case *sdcpb.TypedValue_IdentityrefVal:
		value = updValue.GetIdentityrefVal().Value
	}
	if value == nil && len(jsondata) != 0 {
		err := json.Unmarshal(jsondata, &value)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func ToGNMITypedValue(v *sdcpb.TypedValue) *gnmi.TypedValue {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *sdcpb.TypedValue_AnyVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
		}
	case *sdcpb.TypedValue_AsciiVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
		}
	case *sdcpb.TypedValue_BoolVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
		}
	case *sdcpb.TypedValue_BytesVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
		}
	// case *sdcpb.TypedValue_DecimalVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
	// 	}
	// case *sdcpb.TypedValue_FloatVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
	// 	}
	case *sdcpb.TypedValue_IntVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_IntVal{IntVal: v.GetIntVal()},
		}
	case *sdcpb.TypedValue_JsonIetfVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
		}
	case *sdcpb.TypedValue_JsonVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
		}
	case *sdcpb.TypedValue_LeaflistVal:
		gnmilf := &gnmi.ScalarArray{
			Element: make([]*gnmi.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
		}
		for _, e := range v.GetLeaflistVal().GetElement() {
			gnmilf.Element = append(gnmilf.Element, ToGNMITypedValue(e))
		}
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_LeaflistVal{LeaflistVal: gnmilf},
		}
	case *sdcpb.TypedValue_ProtoBytes:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
		}
	case *sdcpb.TypedValue_StringVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetStringVal()},
		}
	case *sdcpb.TypedValue_UintVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_UintVal{UintVal: v.GetUintVal()},
		}
	case *sdcpb.TypedValue_IdentityrefVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetIdentityrefVal().Value},
		}
	}
	return nil
}

func ParseDecimal64(v string) (*sdcpb.Decimal64, error) {
	// Remove any leading or trailing spaces.
	trimmed := strings.TrimSpace(v)

	if len(trimmed) == 0 {
		return nil, nil
	}

	// Split the string into integer and fractional parts.
	parts := strings.SplitN(trimmed, ".", 2)
	intPart := parts[0]
	var fracPart string
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	// Combine integer and fractional parts into one number.
	combined := intPart + fracPart

	// Parse combined parts into a uint64.
	digits, err := strconv.ParseInt(combined, 10, 64)
	if err != nil {
		return nil, err
	}

	// Calculate precision as the length of the fractional part.
	precision := uint32(len(fracPart))

	// Return the Decimal64 representation.
	return &sdcpb.Decimal64{
		Digits:    digits,
		Precision: precision,
	}, nil
}

// BoolPtr retrieve a pointer to a bool value
func BoolPtr(b bool) *bool {
	return &b
}

func SdcpbUpdateToCacheUpdate(upd *sdcpb.Update, owner string, prio int32) (*cache.Update, error) {
	b, err := proto.Marshal(upd.Value)
	if err != nil {
		return nil, err
	}
	return cache.NewUpdate(utils.ToStrings(upd.GetPath(), false, false), b, prio, owner, 0), nil
}

func SdcpbUpdatesToCacheUpdates(upds []*sdcpb.Update, owner string, prio int32) ([]*cache.Update, error) {
	result := []*cache.Update{}
	for _, upd := range upds {
		cUpd, err := SdcpbUpdateToCacheUpdate(upd, owner, prio)
		if err != nil {
			return nil, err
		}
		result = append(result, cUpd)
	}

	return result, nil
}
