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
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestEqualTypedValues(t *testing.T) {

	// helper to build an AnyVal
	anyVal := func(v []byte) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_AnyVal{AnyVal: &anypb.Any{
				Value: v,
			}},
		}
	}

	// helper to build an IdentityrefVal
	identVal := func(value string, module string, prefix string) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_IdentityrefVal{
				IdentityrefVal: &sdcpb.IdentityRef{
					Value:  value,
					Prefix: prefix,
					Module: module,
				},
			},
		}
	}

	// helper to build a DecimalVal
	decVal := func(digits int64, precision uint32) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DecimalVal{
				DecimalVal: &sdcpb.Decimal64{
					Digits:    digits,
					Precision: precision,
				},
			},
		}
	}

	// helper to build a LeaflistVal
	leaflist := func(elems ...*sdcpb.TypedValue) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{
					Element: elems,
				},
			},
		}
	}

	tests := []struct {
		name string
		t1   *sdcpb.TypedValue
		t2   *sdcpb.TypedValue
		want bool
	}{
		// --- nil cases ---
		{
			"both nil",
			nil,
			nil,
			true,
		},
		{
			"nil, non-nil",
			nil,
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			false,
		},
		{
			"non-nil, nil",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			nil,
			false,
		},

		// --- mismatched types ---
		{
			"ascii vs bool",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: "x"}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			false,
		},

		// --- AnyVal ---
		{
			"any equal",
			anyVal([]byte{1, 2, 3}),
			anyVal([]byte{1, 2, 3}),
			true,
		},
		{
			"any diff",
			anyVal([]byte{1}),
			anyVal([]byte{2}), false,
		},

		// --- AsciiVal ---
		{
			"ascii equal",
			&sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: "foo"},
			},
			&sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: "foo"},
			},
			true,
		},
		{
			"ascii diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: "a"}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: "b"}},
			false,
		},

		// --- IdentityrefVal ---
		{
			"ident equal",
			identVal("v", "m", "p"),
			identVal("v", "m", "p"),
			true,
		},
		{
			"ident diff value",
			identVal("v1", "m", "p"),
			identVal("v2", "m", "p"),
			false,
		},

		// --- BoolVal ---
		{
			"bool equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			true,
		},
		{
			"bool diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}},
			false,
		},

		// --- BytesVal ---
		{
			"bytes equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BytesVal{BytesVal: []byte{1, 2}}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BytesVal{BytesVal: []byte{1, 2}}},
			true,
		},
		{
			"bytes diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BytesVal{BytesVal: []byte{1, 2}}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_BytesVal{BytesVal: []byte{2, 3}}},
			false,
		},

		// --- DecimalVal ---
		{
			"decimal equal",
			decVal(123, 2),
			decVal(123, 2),
			true,
		},
		{
			"decimal precision diff",
			decVal(123, 2),
			decVal(123, 3),
			false,
		},
		{
			"decimal digits diff",
			decVal(123, 2),
			decVal(321, 2),
			false,
		},

		// --- EmptyVal ---
		{
			"empty vs empty",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}},
			true,
		},
		{
			"empty vs non-empty",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_EmptyVal{}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}},
			false,
		},

		// --- FloatVal ---
		{
			"float equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_FloatVal{FloatVal: 1.23}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_FloatVal{FloatVal: 1.23}},
			true,
		},
		{
			"float diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_FloatVal{FloatVal: 1.2}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_FloatVal{FloatVal: 2.1}},
			false,
		},

		// --- IntVal ---
		{
			"int equal pos",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
			true,
		},
		{
			"int equal neg",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: -1}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: -1}},
			true,
		},
		{
			"int diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			false,
		},

		// --- JsonIetfVal ---
		{
			"json-ietf equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"a":1}`)}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"a":1}`)}},
			true,
		},
		{
			"json-ietf diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"a":1}`)}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"a":2}`)}},
			false,
		},

		// --- JsonVal ---
		{
			"json equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"foo":"bar"}`)}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"foo":"bar"}`)}},
			true,
		},
		{
			"json diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"foo":"bar"}`)}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: []byte(`{"bar":"foo"}`)}},
			false,
		},

		// --- LeaflistVal ---
		{
			"leaflist equal same order",
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			),
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			),
			true,
		},
		{
			"leaflist equal diff order",
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
			),
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			),
			true,
		},
		{
			"leaflist diff length",
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
			),
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			),
			false,
		},
		{
			"leaflist diff elements",
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 1}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 2}},
			),
			leaflist(
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 3}},
				&sdcpb.TypedValue{Value: &sdcpb.TypedValue_IntVal{IntVal: 4}},
			),
			false,
		},

		// --- ProtoBytes ---
		{
			"proto bytes equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_ProtoBytes{ProtoBytes: []byte{1, 2}}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_ProtoBytes{ProtoBytes: []byte{1, 2}}},
			true,
		},
		{
			"proto bytes diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_ProtoBytes{ProtoBytes: []byte{1}}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_ProtoBytes{ProtoBytes: []byte{2}}},
			false,
		},

		// --- StringVal ---
		{
			"string equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}},
			true,
		},
		{
			"string diff",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "foo"}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "bar"}},
			false,
		},

		// --- UintVal ---
		{
			"uint equal",
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 1}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 1}},
			true,
		},
		{
			"uint diff", &sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 1}},
			&sdcpb.TypedValue{Value: &sdcpb.TypedValue_UintVal{UintVal: 2}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.t1.Equal(tt.t2)
			if got != tt.want {
				t.Errorf("EqualTypedValues(%v, %v) = %v, want %v", tt.t1, tt.t2, got, tt.want)
			}
		})
	}
}
