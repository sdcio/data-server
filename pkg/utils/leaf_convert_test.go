package utils

import (
	"reflect"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func TestXMLRegexConvert(t *testing.T) {

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "anchors become literals",
			in:   `^\d+$`,
			want: `\^\d+\$`,
		},
		{
			name: "already-escaped anchors stay escaped",
			in:   `foo\$bar`,
			want: `foo\$bar`,
		},
		{
			name: "caret in char class is left alone, dollar is escaped",
			in:   `[^\w]+$`,
			want: `[^\w]+\$`,
		},
		{
			name: "caret later inside char class is escaped",
			in:   `[a^b]`,
			want: `[a\^b]`,
		},
		{
			name: "caret escaped inside char class is escaped",
			in:   `[\^]`,
			want: `[\^]`,
		},
		{
			name: "caret in char class multiple times, dollar is escaped",
			in:   `[^a^b]`,
			want: `[^a\^b]`,
		},
		{
			name: "anchors preceded by a single back-slash stay escaped",
			in:   `\^test\$`,
			want: `\^test\$`,
		},
		{
			name: "empty string",
			in:   ``,
			want: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := XMLRegexConvert(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("XMLRegexConvert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateBitString(t *testing.T) {
	ref := []*sdcpb.Bit{
		{Name: "a", Position: 0},
		{Name: "b", Position: 0},
		{Name: "c", Position: 0},
	}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", true},
		{"exact match", "a b c", true},
		{"skipped middle", "a c", true},
		{"single first element", "a", true},
		{"single last element", "c", true},
		{"out of order", "a c b", false},
		{"wrong order overall", "b c a", false},
		{"unknown single element", "d", false},
		{"unknown element in valid", "a c d", false},
		{"duplicate token", "a a", false},
		{"leading / trailing spaces", "  a   b  ", true},
		{"input longer than schema", "a b c d", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateBitString(tc.input, ref)
			if got != tc.want {
				t.Fatalf("validateBitString(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestConvertBits(t *testing.T) {
	slt := &sdcpb.SchemaLeafType{
		Bits: []*sdcpb.Bit{{Name: "a", Position: 0}, {Name: "b", Position: 1}, {Name: "c", Position: 2}},
	}
	sltEmpty := &sdcpb.SchemaLeafType{}

	sTv := func(s string) *sdcpb.TypedValue {
		return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: s}}
	}

	type inStruct struct {
		value string
		slt   *sdcpb.SchemaLeafType
	}

	tests := []struct {
		name    string
		input   inStruct
		want    *sdcpb.TypedValue
		wantErr bool
	}{
		{
			"valid value",
			inStruct{"a b c", slt},
			sTv("a b c"),
			false,
		},
		{
			"invalid value",
			inStruct{"c b a", slt},
			nil,
			true,
		},
		{"empty schema, empty input",
			inStruct{"", sltEmpty},
			nil,
			true,
		},
		{"empty schema, non-empty input",
			inStruct{"a", sltEmpty},
			nil,
			true,
		},
		{"nil schema pointer",
			inStruct{"a", nil},
			nil,
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertBits(tc.input.value, tc.input.slt)
			if tc.wantErr && err == nil {
				t.Fatalf("wanted error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("wanted no error, got %v", err)
			}
			if !proto.Equal(got, tc.want) {
				t.Fatalf("ConvertBits(%q, %q) = %v, want %v", tc.input.value, tc.input.slt, got, tc.want)
			}
		})
	}
}
