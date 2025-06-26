package utils

import (
	"reflect"
	"testing"
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
