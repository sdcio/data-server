package api_test

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/api"
)

func TestEntryMapSortedKeysToleratesEmpty(t *testing.T) {
	var empty api.EntryMap
	if got := empty.SortedKeys(); len(got) != 0 {
		t.Fatalf("nil EntryMap.SortedKeys() = %v, want empty", got)
	}
	if got := (api.EntryMap{}).SortedKeys(); len(got) != 0 {
		t.Fatalf("empty EntryMap.SortedKeys() = %v, want empty", got)
	}
}
