package tree

import (
	"sync"
	"testing"
)

func Test_childMap_DeleteChilds(t *testing.T) {
	type fields struct {
		c map[string]Entry
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
				c: map[string]Entry{
					"one": &sharedEntryAttributes{
						pathElemName: "one",
					},
					"two": &sharedEntryAttributes{
						pathElemName: "two",
					},
					"three": &sharedEntryAttributes{
						pathElemName: "three",
					},
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
				c: map[string]Entry{
					"one": &sharedEntryAttributes{
						pathElemName: "one",
					},
					"two": &sharedEntryAttributes{
						pathElemName: "two",
					},
					"three": &sharedEntryAttributes{
						pathElemName: "three",
					},
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
				c: map[string]Entry{
					"one": &sharedEntryAttributes{
						pathElemName: "one",
					},
					"two": &sharedEntryAttributes{
						pathElemName: "two",
					},
					"three": &sharedEntryAttributes{
						pathElemName: "three",
					},
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
			c := &childMap{
				c:  tt.fields.c,
				mu: sync.RWMutex{},
			}
			c.DeleteChilds(tt.args.names)
			if len(c.c) != tt.expectedLength {
				t.Errorf("expected %d elements got %d", tt.expectedLength, len(c.c))
			}
		})
	}
}

func Test_childMap_DeleteChild(t *testing.T) {
	type fields struct {
		c map[string]Entry
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
				c: map[string]Entry{
					"one": &sharedEntryAttributes{
						pathElemName: "one",
					},
					"two": &sharedEntryAttributes{
						pathElemName: "two",
					},
					"three": &sharedEntryAttributes{
						pathElemName: "three",
					},
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
				c: map[string]Entry{
					"one": &sharedEntryAttributes{
						pathElemName: "one",
					},
					"two": &sharedEntryAttributes{
						pathElemName: "two",
					},
					"three": &sharedEntryAttributes{
						pathElemName: "three",
					},
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
			c := &childMap{
				c:  tt.fields.c,
				mu: sync.RWMutex{},
			}
			c.DeleteChild(tt.args.name)
			if len(c.c) != tt.expectedLength {
				t.Errorf("expected %d elements got %d", tt.expectedLength, len(c.c))
			}
		})
	}
}
