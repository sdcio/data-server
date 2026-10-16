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
		ids []api.NodeIdentity
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
				ids: []api.NodeIdentity{api.LocalIdentity("one")},
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
				ids: []api.NodeIdentity{api.LocalIdentity("three"), api.LocalIdentity("one")},
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
				ids: []api.NodeIdentity{api.LocalIdentity("four")},
			},
			expectedLength: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := api.NewChildMapWithEntries(tt.fields.c)
			c.DeleteChilds(tt.args.ids)
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
		id api.NodeIdentity
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
				id: api.LocalIdentity("three"),
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
				id: api.LocalIdentity("four"),
			},
			expectedLength: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := api.NewChildMapWithEntries(tt.fields.c)
			c.DeleteChild(tt.args.id)
			if c.Length() != tt.expectedLength {
				t.Errorf("expected %d elements got %d", tt.expectedLength, c.Length())
			}
		})
	}
}

func Test_childMap_AddOrGet_collidingLocalNames(t *testing.T) {
	modA := api.NodeIdentity{Local: "router", Module: "mod-a"}
	modB := api.NodeIdentity{Local: "router", Module: "mod-b"}

	c := api.NewChildMapWithEntries(map[string]api.Entry{
		modA.MapKey(): nil,
		modB.MapKey(): nil,
	})
	if c.Length() != 2 {
		t.Fatalf("expected 2 children, got %d", c.Length())
	}
	if _, ok := c.GetEntry(modA); !ok {
		t.Fatal("mod-a router missing")
	}
	if _, ok := c.GetEntry(modB); !ok {
		t.Fatal("mod-b router missing")
	}
}
