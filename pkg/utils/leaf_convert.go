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
	"fmt"
	"strconv"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func ConvertJsonValueToTv(d any, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	var err error
	switch slt.Type {
	case "string":
		v, ok := d.(string)
		if !ok {
			return nil, fmt.Errorf("error converting %v to string", v)
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{StringVal: v},
		}, nil
	case "leafref":
		return ConvertJsonValueToTv(d, slt.LeafrefTargetType)
	case "identityref":
		v, ok := d.(string)
		if !ok {
			return nil, fmt.Errorf("error converting %v to string", v)
		}
		return sdcpb.TVFromString(slt, v, 0)
	case "uint64", "uint32", "uint16", "uint8":
		var i uint64
		switch v := d.(type) {
		case string: // the 64 bit types are transported as strings in json
			i, err = strconv.ParseUint(d.(string), 10, 64)
			if err != nil {
				return nil, err
			}
		case uint8:
			i = uint64(v)
		case uint16:
			i = uint64(v)
		case uint32:
			i = uint64(v)
		case uint64:
			i = v
		case float64:
			i = uint64(v)
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_UintVal{UintVal: i},
		}, nil

	case "int64", "int32", "int16", "int8":
		var i int64
		switch v := d.(type) {
		case string: // the 64 bit types are transported as strings in json
			i, err = strconv.ParseInt(d.(string), 10, 64)
			if err != nil {
				return nil, err
			}
		case int8:
			i = int64(v)
		case int16:
			i = int64(v)
		case int32:
			i = int64(v)
		case int64:
			i = v
		case float64:
			i = int64(v)
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_IntVal{IntVal: i},
		}, nil
	case "boolean":
		var b bool
		switch d := d.(type) {
		case bool:
			b = d
		case string:
			b, err = strconv.ParseBool(d)
			if err != nil {
				return nil, err
			}
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_BoolVal{BoolVal: b},
		}, nil
	case "decimal64":
		arr := strings.SplitN(d.(string), ".", 2)
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
	case "union":
		for _, ut := range slt.GetUnionTypes() {
			tv, err := ConvertJsonValueToTv(d, ut)
			if err == nil {
				return tv, nil
			}
		}
		return nil, fmt.Errorf("invalid value %s for union type: %v", d, slt.GetUnionTypes())
	case "enumeration":
		// TODO: get correct type, assuming string
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{StringVal: fmt.Sprintf("%v", d)},
		}, nil
	case "empty":
		return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{EmptyVal: &emptypb.Empty{}}}, nil
	case "bits":
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{StringVal: fmt.Sprintf("%v", d)},
		}, nil
	}

	return nil, fmt.Errorf("error no case matched when converting from json to TV: %v, %v", d, slt)
}
