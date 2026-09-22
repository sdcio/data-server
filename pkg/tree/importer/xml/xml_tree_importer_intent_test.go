package xml

import (
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/importer"
)

// TestXmlTreeImporter_NotIntentAdapter is the deletion test for ticket 03:
// XML importers must satisfy ImportConfigAdapter without gaining Intent-only
// metadata. If Orphan/SensitivePaths land back on ImportConfigAdapter (or are
// stubbed here again), this fails.
func TestXmlTreeImporter_NotIntentAdapter(t *testing.T) {
	root := etree.NewElement("root")
	var adapter importer.ImportConfigAdapter = NewXmlTreeImporter(root, "running", 0, false)
	if _, ok := adapter.(importer.IntentAdapter); ok {
		t.Fatal("XmlTreeImporter must not implement IntentAdapter")
	}
}
