package json

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/importer"
)

// TestJsonTreeImporter_NotIntentAdapter: a JSON importer satisfies
// ImportConfigAdapter and does not implement IntentAdapter. Orphan and
// SensitivePaths stay on intent reads; JSON carries device and running data.
func TestJsonTreeImporter_NotIntentAdapter(t *testing.T) {
	var adapter importer.ImportConfigAdapter = NewJsonTreeImporter(map[string]any{}, "running", 0, false)
	if _, ok := adapter.(importer.IntentAdapter); ok {
		t.Fatal("JsonTreeImporter must not implement IntentAdapter")
	}
}
