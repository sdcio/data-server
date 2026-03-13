package api_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
)


func Test_childMap_DeleteChilds(t *testing.T) {
	type fields struct {
		c map[string]api.Entry
	}
	type args struct {
		names []string
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		expectedLength int
	}{
		{
			name: "Delete single entry",
			fields: fields{
				c: map[string]api.Entry{
					"one":   nil,
					"two":   nil,
					"three": nil,
				},
			},
			args: args{
				names: []string{"one"},
			},
			expectedLength: 2,
		},
		{
			name: "Delete two entries",
			fields: fields{
				c: map[string]api.Entry{
					"one":   nil,
					"two":   nil,
					"three": nil,
				},
			},
			args: args{
				names: []string{"three", "one"},
			},
			expectedLength: 1,
		},
		{
			name: "Delete non-existing entry",
			fields: fields{
				c: map[string]api.Entry{
					"one":   nil,
					"two":   nil,
					"three": nil,
				},
			},
			args: args{
				names: []string{"four"},
			},
			expectedLength: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := api.NewChildMapWithEntries(tt.fields.c)
			c.DeleteChilds(tt.args.names)
			if c.Length() != tt.expectedLength {
				t.Errorf("expected %d elements got %d", tt.expectedLength, c.Length())
			}
		})
	}
}

func Test_childMap_DeleteChild(t *testing.T) {
	type fields struct {
		c map[string]api.Entry
	}
	type args struct {
		name string
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		expectedLength int
	}{
		{
			name: "Delete existing entry",
			fields: fields{
				c: map[string]api.Entry{
					"one":   nil,
					"two":   nil,
					"three": nil,
				},
			},
			args: args{
				name: "three",
			},
			expectedLength: 2,
		},
		{
			name: "Delete non-existing entry",
			fields: fields{
				c: map[string]api.Entry{
					"one":   nil,
					"two":   nil,
					"three": nil,
				},
			},
			args: args{
				name: "four",
			},
			expectedLength: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := api.NewChildMapWithEntries(tt.fields.c)
			c.DeleteChild(tt.args.name)
			if c.Length() != tt.expectedLength {
				t.Errorf("expected %d elements got %d", tt.expectedLength, c.Length())
			}
		})
	}
}
