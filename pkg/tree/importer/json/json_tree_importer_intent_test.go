package json

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/importer"
)

// TestJsonTreeImporter_NotIntentAdapter is the deletion test for ticket 03:
// JSON importers must satisfy ImportConfigAdapter without gaining Intent-only
// metadata. If Orphan/SensitivePaths land back on ImportConfigAdapter (or are
// stubbed here again), this fails.
func TestJsonTreeImporter_NotIntentAdapter(t *testing.T) {
	var adapter importer.ImportConfigAdapter = NewJsonTreeImporter(map[string]any{}, "running", 0, false)
	if _, ok := adapter.(importer.IntentAdapter); ok {
		t.Fatal("JsonTreeImporter must not implement IntentAdapter")
	}
}
