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
	"math"
	"regexp"
	"strconv"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

func Convert(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	if lst == nil {
		return nil, fmt.Errorf("schemaleaftype cannot be nil")
	}
	switch lst.Type {
	case "string":
		return ConvertString(value, lst)
	case "union":
		return ConvertUnion(value, lst.UnionTypes)
	case "boolean":
		return ConvertBoolean(value, lst)
	case "int8":
		// TODO: HEX and OCTAL pre-processing for all INT types
		// https://www.rfc-editor.org/rfc/rfc6020.html#page-112
		return ConvertInt8(value, lst)
	case "int16":
		return ConvertInt16(value, lst)
	case "int32":
		return ConvertInt32(value, lst)
	case "int64":
		return ConvertInt64(value, lst)
	case "uint8":
		return ConvertUint8(value, lst)
	case "uint16":
		return ConvertUint16(value, lst)
	case "uint32":
		return ConvertUint32(value, lst)
	case "uint64":
		// TODO: fraction-digits (https://www.rfc-editor.org/rfc/rfc6020.html#section-9.3.4)
		return ConvertUint64(value, lst)
	case "enumeration":
		return ConvertEnumeration(value, lst)
	case "empty":
		return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}}, nil
	case "bits":
		return ConvertBits(value, lst)
	case "binary": // https://www.rfc-editor.org/rfc/rfc6020.html#section-9.8
		return ConvertBinary(value, lst)
	case "leafref": // https://www.rfc-editor.org/rfc/rfc6020.html#section-9.9
		// leafrefs are being treated as strings.
		// further validation needs to happen later in the process
		return ConvertLeafRef(value, lst)
	case "identityref": //TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.10
		return ConvertIdentityRef(value, lst)
	case "instance-identifier": //TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.13
		return ConvertInstanceIdentifier(value, lst)
	case "decimal64":
		return ConvertDecimal64(value, lst)
	}
	log.Warnf("type %q not implemented", lst.Type)
	return ConvertString(value, lst)
}

func ConvertInstanceIdentifier(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// delegate to string, validation is left for a different party at a later stage in processing
	return ConvertString(value, slt)
}

func ConvertIdentityRef(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return convertStringToTv(slt, value, 0)
}

func ConvertBinary(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// Binary is basically a base64 encoded string that might carry a length restriction
	// so we should be fine with delegating to string
	return ConvertString(value, slt)
}

func ConvertLeafRef(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// a leafref should basically be a string value that also exists somewhere else in the config as a value.
	// we leave the validation of the leafrefs to a different party at a later stage
	return ConvertString(value, slt)
}

func ConvertEnumeration(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// iterate the valid values as per schema
	for _, item := range slt.EnumNames {
		// if value is found, return a StringVal
		if value == item {
			return &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_StringVal{
					StringVal: value,
				},
			}, nil
		}
	}
	// If value is not found return an error
	return nil, fmt.Errorf("value %q does not match any valid enum values [%s]", value, strings.Join(slt.EnumNames, ", "))
}

func ConvertBoolean(value string, _ *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	var bval bool
	// check for true or false in string representation
	switch value {
	case "true":
		bval = true
	case "false":
		bval = false
	default:
		// if it is any other value, return error
		return nil, fmt.Errorf("illegal value %q for boolean type", value)
	}
	// otherwise return the BoolVal TypedValue
	return &sdcpb.TypedValue{
		Value: &sdcpb.TypedValue_BoolVal{
			BoolVal: bval,
		},
	}, nil
}

func ConvertSdcpbNumberToUint64(mm *sdcpb.Number) (uint64, error) {
	if mm.Negative {
		return 0, fmt.Errorf("negative number to uint conversion")
	}
	return mm.Value, nil
}

func intAbs(x int64) uint64 {
	ui := uint64(x)
	if x < 0 {
		return ^(ui) + 1
	}
	return ui
}

func ConvertSdcpbNumberToInt64(mm *sdcpb.Number) (int64, error) {
	if mm.Negative {
		if mm.Value > intAbs(math.MinInt64) {
			return 0, fmt.Errorf("error converting -%d to int64: overflow", mm.Value)
		}
		return -int64(mm.Value), nil
	}

	if mm.Value > math.MaxInt64 {
		return 0, fmt.Errorf("error converting %d to int64 overflow", mm.Value)
	}
	return int64(mm.Value), nil
}

func convertUint(value string, minMaxs []*sdcpb.SchemaMinMaxType, ranges *URnges) (*sdcpb.TypedValue, error) {
	if ranges == nil {
		ranges = NewUrnges()
	}
	for _, x := range minMaxs {
		min, err := ConvertSdcpbNumberToUint64(x.Min)
		if err != nil {
			return nil, err
		}
		max, err := ConvertSdcpbNumberToUint64(x.Max)
		if err != nil {
			return nil, err
		}
		ranges.AddRange(min, max)
	}

	// validate the value against the ranges
	val, err := ranges.IsWithinAnyRangeString(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint8(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewUrnges()
	ranges.AddRange(0, math.MaxUint8)

	return convertUint(value, lst.Range, ranges)
}

func ConvertUint16(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewUrnges()
	ranges.AddRange(0, math.MaxUint16)

	return convertUint(value, lst.Range, ranges)
}

func ConvertUint32(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewUrnges()
	ranges.AddRange(0, math.MaxUint32)

	return convertUint(value, lst.Range, ranges)
}

func ConvertUint64(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewUrnges()

	return convertUint(value, lst.Range, ranges)
}

func convertInt(value string, minMaxs []*sdcpb.SchemaMinMaxType, ranges *SRnges) (*sdcpb.TypedValue, error) {
	for _, x := range minMaxs {
		min, err := ConvertSdcpbNumberToInt64(x.Min)
		if err != nil {
			return nil, err
		}
		max, err := ConvertSdcpbNumberToInt64(x.Max)
		if err != nil {
			return nil, err
		}
		ranges.AddRange(min, max)
	}

	// validate the value against the ranges
	val, err := ranges.IsWithinAnyRangeString(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt8(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewSrnges()
	ranges.AddRange(math.MinInt8, math.MaxInt8)

	return convertInt(value, lst.Range, ranges)
}

func ConvertInt16(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewSrnges()
	ranges.AddRange(math.MinInt16, math.MaxInt16)

	return convertInt(value, lst.Range, ranges)
}

func ConvertInt32(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewSrnges()
	ranges.AddRange(math.MinInt32, math.MaxInt32)

	return convertInt(value, lst.Range, ranges)
}
func ConvertInt64(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create the ranges
	ranges := NewSrnges()

	return convertInt(value, lst.Range, ranges)
}

func XMLRegexConvert(s string) string {

	cTest := func(r rune, prev rune) bool {
		// if ^ is not following a [ or if $ we want to return true
		return (r == '^' && prev != '[') || r == '$'
	}

	b := strings.Builder{}
	b.Grow(len(s) + len(s)/4)
	slashes := 0
	prevR := rune(0)

	for _, r := range s {
		if r == '\\' {
			slashes++
			prevR = r
			b.WriteRune(r)
			continue
		}

		if cTest(r, prevR) && slashes%2 == 0 {
			b.WriteRune('\\')
		}

		slashes = 0
		prevR = r
		b.WriteRune(r)
	}
	return b.String()
}

func ConvertString(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// check length of the string if the length property is set
	// length will contain a range like string definition "5..60" or "7..10|40..45"
	if len(lst.Length) != 0 {
		_, err := convertUint(strconv.Itoa(len(value)), lst.Length, nil)

		if err != nil {
			return nil, err
		}

	}

	overallMatch := true
	// If the type has multiple "pattern" statements, the expressions are
	// ANDed together, i.e., all such expressions have to match.
	for _, sp := range lst.Patterns {
		// The set of metacharacters is not the same between XML schema and perl/python/go REs
		// the set of metacharacters for XML is: .\?*+{}()[] (https://www.w3.org/TR/xmlschema-2/#dt-metac)
		// the set of metacharacters defined in go is: \.+*?()|[]{}^$ (go/libexec/src/regexp/regexp.go:714)
		// we need therefore to escape some values
		// TODO check about '^'

		escaped := XMLRegexConvert(sp.Pattern)
		re, err := regexp.Compile(escaped)
		if err != nil {
			log.Errorf("unable to compile regex %q", sp.Pattern)
		}
		match := re.MatchString(value)
		// if it is a match and not inverted
		// or it is not a match but inverted
		// then this is valid
		if (match && !sp.Inverted) || (!match && sp.Inverted) {
			continue
		} else {
			overallMatch = false
			break
		}
	}
	if overallMatch {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{
				StringVal: value,
			},
		}, nil
	}
	return nil, fmt.Errorf("%q does not match patterns", value)

}

func ConvertDecimal64(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	d64, err := ParseDecimal64(value)
	if err != nil {
		return nil, err
	}

	return &sdcpb.TypedValue{
		Value: &sdcpb.TypedValue_DecimalVal{
			DecimalVal: d64,
		},
	}, nil
}

func ConvertUnion(value string, slts []*sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// iterate over the union types try to convert without error
	for _, slt := range slts {
		tv, err := Convert(value, slt)
		// if no error type conversion was fine
		if err != nil {
			continue
		}
		// return the TypedValue
		return tv, nil
	}
	return nil, fmt.Errorf("no union type fit the provided value %q", value)
}

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
		return convertStringToTv(slt, v, 0)
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
		switch d.(type) {
		case bool:
			b = d.(bool)
		case string:
			b, err = strconv.ParseBool(d.(string))
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

func validateBitString(value string, allowed []*sdcpb.Bit) bool {
	//split string to individual bits
	bits := strings.Fields(value)
	// empty string is fine
	if len(bits) == 0 {
		return true
	}
	// track pos inside allowed slice
	pos := 0
	for _, b := range bits {
		// increase pos until we get to an allowed bit or we reach the end of the slice
		for pos < len(allowed) && allowed[pos].GetName() != b {
			pos++
		}
		// if we are at the end of the array, we did not validate
		if pos == len(allowed) {
			return false
		}
		//move past found element
		pos++
	}
	return true
}

func ConvertBits(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	if slt == nil {
		return nil, fmt.Errorf("type information is nil")
	}
	if len(slt.Bits) == 0 {
		return nil, fmt.Errorf("type information is missing bits information")
	}
	if validateBitString(value, slt.Bits) {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{
				StringVal: value,
			},
		}, nil
	}
	// If value is not valid return an error
	validBits := make([]string, 0, len(slt.Bits))
	for _, b := range slt.Bits {
		validBits = append(validBits, b.GetName())
	}
	return nil, fmt.Errorf("value %q does not follow required bit ordering [%s]", value, strings.Join(validBits, " "))
}
