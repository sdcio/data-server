package xml

import (
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/importer"
)

// TestXmlTreeImporter_NotIntentAdapter: an XML importer satisfies
// ImportConfigAdapter and does not implement IntentAdapter. Orphan and
// SensitivePaths stay on intent reads; XML carries device and running data.
func TestXmlTreeImporter_NotIntentAdapter(t *testing.T) {
	root := etree.NewElement("root")
	var adapter importer.ImportConfigAdapter = NewXmlTreeImporter(root, "running", 0, false)
	if _, ok := adapter.(importer.IntentAdapter); ok {
		t.Fatal("XmlTreeImporter must not implement IntentAdapter")
	}
}
