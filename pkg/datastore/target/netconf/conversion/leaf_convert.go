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

package conversion

import (
	"encoding/base64"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

func Convert(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
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
		// TODO https://www.rfc-editor.org/rfc/rfc6020.html#section-9.11
		// I'm not sure what should be returned atm.
		// KR:use an empty string for a leaf of type "empty"
		//    probably need to add typedValue of type Empty in the protos
		return ConvertString(value, lst)
	case "bits":
	// TODO: https://www.rfc-editor.org/rfc/rfc6020.html#section-9.7
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
	// delegate to string, validation is left for a different party at a later stage in processing
	return ConvertString(value, slt)
}

func ConvertBinary(value string, blt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		log.Fatalf("error decoding base64 string: %v", err)
	}

	if len(blt.Length) != 0 {
		r := NewUrnges(blt.Length, 0, math.MaxUint64)

		inLength := false
		for _, rng := range r.rnges {
			if rng.isInRange(uint64(len(data))) {
				inLength = true
				break
			}
		}
		if !inLength {
			// prepare log message, collecting string rep of the ranges
			str_ranges := []string{}
			for _, x := range r.rnges {
				str_ranges = append(str_ranges, x.String())
			}
			return nil, fmt.Errorf("length of the value (%d) is not within the schema defined ranges [ %s ]", len(value), strings.Join(str_ranges, ", "))
		}
	}

	return &sdcpb.TypedValue{
		Value: &sdcpb.TypedValue_BytesVal{
			BytesVal: data,
		},
	}, nil
}

func ConvertLeafRef(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// a leafref should basically be a string value that also exists somewhere else in the config as a value.
	// we leave the validation of the leafrefs to a different party at a later stage
	return ConvertString(value, slt)
}

func ConvertEnumeration(value string, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// iterate the valid values as per schema
	for _, item := range slt.Values {
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
	return nil, fmt.Errorf("value %q does not match any valid enum values [%s]", value, strings.Join(slt.Values, ", "))
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
func ConvertUint8(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint8)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint16(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint16)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint32(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint32)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertUint64(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewUrnges(lst.Range, 0, math.MaxUint64)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt8(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt8, math.MaxInt8)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt16(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt16, math.MaxInt16)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertInt32(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt32, math.MaxInt32)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}
func ConvertInt64(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// create / parse the ranges
	ranges := NewSrnges(lst.Range, math.MinInt64, math.MaxInt64)
	// validate the value against the ranges
	val, err := ranges.isWithinAnyRange(value)
	if err != nil {
		return nil, err
	}
	// return the TypedValue
	return val, nil
}

func ConvertString(value string, lst *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	// check length of the string if the length property is set
	// length will contain a range like string definition "5..60" or "7..10|40..45"
	if len(lst.Length) != 0 {
		r := NewUrnges(lst.Length, 0, math.MaxUint64)

		inLength := false
		for _, rng := range r.rnges {
			if rng.isInRange(uint64(len(value))) {
				inLength = true
			}
		}
		if !inLength {
			// prepare log message, collecting string rep of the ranges
			str_ranges := []string{}
			for _, x := range r.rnges {
				str_ranges = append(str_ranges, x.String())
			}
			return nil, fmt.Errorf("length of the value (%d) is not within the schema defined ranges [ %s ]", len(value), strings.Join(str_ranges, ", "))
		}
	}

	overallMatch := true
	// If the type has multiple "pattern" statements, the expressions are
	// ANDed together, i.e., all such expressions have to match.
	for _, sp := range lst.Patterns {
		re, err := regexp.Compile(sp.Pattern)
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
	d64, err := utils.ParseDecimal64(value)
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
