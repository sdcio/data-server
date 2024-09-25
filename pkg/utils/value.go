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
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/openconfig/gnmi/proto/gnmi"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
	}
	if value == nil && len(jsondata) != 0 {
		err := json.Unmarshal(jsondata, &value)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

func ToSchemaTypedValue(v any) *sdcpb.TypedValue {
	log.Debugf("to schema value %T, %#v", v, v) //TODO2: Writing in sdcpb typedValue, need to change to gNMI ?
	switch v := v.(type) {
	case *sdcpb.TypedValue:
		return v
	case *gnmi.TypedValue:
		return ToSchemaTypedValue(v.GetValue())
	case *gnmi.TypedValue_AnyVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_AnyVal{
				AnyVal: v.AnyVal,
			},
		}
	case *gnmi.TypedValue_AsciiVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_AsciiVal{
				AsciiVal: v.AsciiVal,
			},
		}
	case *gnmi.TypedValue_StringVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{
				StringVal: v.StringVal,
			},
		}
	case *gnmi.TypedValue_BoolVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_BoolVal{
				BoolVal: v.BoolVal,
			},
		}
	case *gnmi.TypedValue_BytesVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_BytesVal{
				BytesVal: v.BytesVal,
			},
		}
	case *gnmi.TypedValue_IntVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_IntVal{
				IntVal: v.IntVal,
			},
		}
	case *gnmi.TypedValue_UintVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_UintVal{
				UintVal: v.UintVal,
			},
		}
	case *gnmi.TypedValue_ProtoBytes:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_ProtoBytes{
				ProtoBytes: v.ProtoBytes,
			},
		}
	case *gnmi.TypedValue_LeaflistVal:
		schemalf := &sdcpb.ScalarArray{
			Element: make([]*sdcpb.TypedValue, 0, len(v.LeaflistVal.GetElement())),
		}
		for _, e := range v.LeaflistVal.GetElement() {
			schemalf.Element = append(schemalf.Element, ToSchemaTypedValue(e))
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: schemalf},
		}
	}
	return nil
}

// func ToGNMITypedValue(v any) *gnmi.TypedValue {
// 	log.Infof("to gNMI value %T, %v", v, v)
// 	switch v := v.(type) {
// 	case *sdcpb.TypedValue_AsciiVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AsciiVal{
// 				AsciiVal: v.AsciiVal,
// 			},
// 		}
// 	case *sdcpb.TypedValue_StringVal:
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
	}
	return nil
}

func EqualTypedValues(v1, v2 *sdcpb.TypedValue) bool {
	if v1 == nil {
		return v2 == nil
	}
	if v2 == nil {
		return v1 == nil
	}

	switch v1 := v1.GetValue().(type) {
	case *sdcpb.TypedValue_AnyVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_AnyVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if v1.AnyVal == nil && v2.AnyVal == nil {
				return true
			}
			if v1.AnyVal == nil || v2.AnyVal == nil {
				return false
			}
			if v1.AnyVal.GetTypeUrl() != v2.AnyVal.GetTypeUrl() {
				return false
			}
			return bytes.Equal(v1.AnyVal.GetValue(), v2.AnyVal.GetValue())
		default:
			return false
		}
	case *sdcpb.TypedValue_AsciiVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_AsciiVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.AsciiVal == v2.AsciiVal
		default:
			return false
		}
	case *sdcpb.TypedValue_BoolVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_BoolVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.BoolVal == v2.BoolVal
		default:
			return false
		}
	case *sdcpb.TypedValue_BytesVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_BytesVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.BytesVal, v2.BytesVal)
		default:
			return false
		}
	case *sdcpb.TypedValue_DecimalVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_DecimalVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if v1.DecimalVal.GetDigits() != v2.DecimalVal.GetDigits() {
				return false
			}
			return v1.DecimalVal.GetPrecision() == v2.DecimalVal.GetPrecision()
		default:
			return false
		}
	case *sdcpb.TypedValue_EmptyVal:
		switch v2.GetValue().(type) {
		case *sdcpb.TypedValue_EmptyVal:
			return true
		default:
			return false
		}
	case *sdcpb.TypedValue_FloatVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_FloatVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.FloatVal == v2.FloatVal
		default:
			return false
		}
	case *sdcpb.TypedValue_IntVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_IntVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.IntVal == v2.IntVal
		default:
			return false
		}
	case *sdcpb.TypedValue_JsonIetfVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_JsonIetfVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.JsonIetfVal, v2.JsonIetfVal)
		default:
			return false
		}
	case *sdcpb.TypedValue_JsonVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_JsonVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.JsonVal, v2.JsonVal)
		default:
			return false
		}
	case *sdcpb.TypedValue_LeaflistVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if len(v1.LeaflistVal.GetElement()) != len(v2.LeaflistVal.GetElement()) {
				return false
			}
			for i := range v1.LeaflistVal.GetElement() {
				if !EqualTypedValues(v1.LeaflistVal.Element[i], v2.LeaflistVal.Element[i]) {
					return false
				}
			}
		default:
			return false
		}
	case *sdcpb.TypedValue_ProtoBytes:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_ProtoBytes:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.ProtoBytes, v2.ProtoBytes)
		default:
			return false
		}
	case *sdcpb.TypedValue_StringVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_StringVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.StringVal == v2.StringVal
		default:
			return false
		}
	case *sdcpb.TypedValue_UintVal:
		switch v2 := v2.GetValue().(type) {
		case *sdcpb.TypedValue_UintVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.UintVal == v2.UintVal
		default:
			return false
		}
	}
	// TODO: Why is this default case to return true??
	return true
}

func TypedValueToString(tv *sdcpb.TypedValue) string {
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_AnyVal:
		return string(tv.GetAnyVal().GetValue()) // questionable...
	case *sdcpb.TypedValue_AsciiVal:
		return tv.GetAsciiVal()
	case *sdcpb.TypedValue_BoolVal:
		return strconv.FormatBool(tv.GetBoolVal())
	case *sdcpb.TypedValue_BytesVal:
		return string(tv.GetBytesVal()) // questionable...
	case *sdcpb.TypedValue_DecimalVal:
		d := tv.GetDecimalVal()
		digitsStr := strconv.FormatInt(d.Digits, 10)
		negative := false
		if d.Digits < 0 {
			negative = true
			digitsStr = digitsStr[1:] // Remove the "-" sign for processing
		}
		// Add leading zeros if necessary
		for uint32(len(digitsStr)) <= d.Precision {
			digitsStr = "0" + digitsStr
		}
		// Insert the decimal point
		if d.Precision > 0 {
			decimalPointIndex := len(digitsStr) - int(d.Precision)
			digitsStr = digitsStr[:decimalPointIndex] + "." + digitsStr[decimalPointIndex:]
		}
		// Add back the negative sign if necessary
		if negative {
			digitsStr = "-" + digitsStr
		}
		return digitsStr
	case *sdcpb.TypedValue_DoubleVal:
		return strconv.FormatFloat(tv.GetDoubleVal(), byte('e'), -1, 64)
	case *sdcpb.TypedValue_EmptyVal:
		return "{}"
	case *sdcpb.TypedValue_FloatVal:
		return strconv.FormatFloat(float64(tv.GetFloatVal()), byte('e'), -1, 64)
	case *sdcpb.TypedValue_IntVal:
		return strconv.Itoa(int(tv.GetIntVal()))
	case *sdcpb.TypedValue_JsonIetfVal:
		return string(tv.GetJsonIetfVal())
	case *sdcpb.TypedValue_JsonVal:
		return string(tv.GetJsonVal())
	case *sdcpb.TypedValue_LeaflistVal:
		rs := make([]string, 0, len(tv.GetLeaflistVal().GetElement()))
		for _, lfv := range tv.GetLeaflistVal().GetElement() {
			rs = append(rs, TypedValueToString(lfv))
		}
		return strings.Join(rs, ",")
	case *sdcpb.TypedValue_ProtoBytes:
		return string(tv.GetProtoBytes()) // questionable
	case *sdcpb.TypedValue_StringVal:
		return tv.GetStringVal()
	case *sdcpb.TypedValue_UintVal:
		return strconv.Itoa(int(tv.GetUintVal()))
	}
	return ""
}

func ParseDecimal64(v string) (*sdcpb.Decimal64, error) {
	// Remove any leading or trailing spaces.
	trimmed := strings.TrimSpace(v)

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
