package api_test

import (
	"fmt"
	"testing"
	"time"

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

func TestChildMapGetAllEmptyDoesNotAllocate(t *testing.T) {
	c := api.NewChildMap()
	allocs := testing.AllocsPerRun(1000, func() {
		got := c.GetAll()
		if len(got) != 0 {
			t.Fatalf("GetAll() len = %d, want 0", len(got))
		}
	})
	if allocs != 0 {
		t.Fatalf("GetAll() on empty ChildMap allocated %.2f times per run, want 0", allocs)
	}
}

func TestChildMapSnapshotEmptyDoesNotAllocate(t *testing.T) {
	c := api.NewChildMap()
	allocs := testing.AllocsPerRun(1000, func() {
		got := c.Snapshot(nil)
		if len(got) != 0 {
			t.Fatalf("Snapshot() len = %d, want 0", len(got))
		}
	})
	if allocs != 0 {
		t.Fatalf("Snapshot() on empty ChildMap allocated %.2f times per run, want 0", allocs)
	}
}

func TestChildMapGetAllSortedEmptyDoesNotAllocate(t *testing.T) {
	c := api.NewChildMap()
	allocs := testing.AllocsPerRun(1000, func() {
		got := c.GetAllSorted()
		if len(got) != 0 {
			t.Fatalf("GetAllSorted() len = %d, want 0", len(got))
		}
	})
	if allocs != 0 {
		t.Fatalf("GetAllSorted() on empty ChildMap allocated %.2f times per run, want 0", allocs)
	}
}

func TestChildMapGetAllSortedDoesNotDeadlockWithConcurrentDelete(t *testing.T) {
	entries := make(map[string]api.Entry, 256)
	for i := 0; i < 256; i++ {
		entries[fmt.Sprintf("%04d", i)] = nil
	}
	c := api.NewChildMapWithEntries(entries)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 5000; i++ {
			_ = c.GetAllSorted()
			_ = c.GetKeys()
			_ = c.SortedKeys()
		}
		close(done)
	}()
	go func() {
		for i := 0; i < 256; i++ {
			c.DeleteChild(fmt.Sprintf("%04d", i))
		}
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("deadlock: child-map key listing nested a read lock while a writer waited")
	}
}
