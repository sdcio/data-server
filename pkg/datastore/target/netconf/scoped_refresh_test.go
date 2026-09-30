package netconf

import (
	"context"
	"testing"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/importer"
	xmlimporter "github.com/sdcio/data-server/pkg/tree/importer/xml"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type stubImportAdapter struct {
	elements []importer.ImportConfigAdapterElement
	deletes  *sdcpb.PathSet
}

func (s stubImportAdapter) GetElements() []importer.ImportConfigAdapterElement { return s.elements }
func (s stubImportAdapter) GetElement(string) importer.ImportConfigAdapterElement { return nil }
func (s stubImportAdapter) GetKeyValue(context.Context, *sdcpb.SchemaLeafType) (string, error) {
	return "", nil
}
func (s stubImportAdapter) GetTVValue(context.Context, *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return nil, nil
}
func (s stubImportAdapter) GetName() string              { return "stub" }
func (s stubImportAdapter) GetDeletes() *sdcpb.PathSet   { return s.deletes }
func (s stubImportAdapter) GetPriority() int32           { return 0 }
func (s stubImportAdapter) GetNonRevertive() bool        { return false }

func TestNetconfScopedImportAdapter(t *testing.T) {
	t.Run("nil stays nil", func(t *testing.T) {
		if got := netconfScopedImportAdapter(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("empty elements and deletes become nil", func(t *testing.T) {
		if got := netconfScopedImportAdapter(stubImportAdapter{deletes: sdcpb.NewPathSet()}); got != nil {
			t.Error("want nil importer for empty snapshot")
		}
	})

	t.Run("non-empty elements preserved", func(t *testing.T) {
		imp := stubImportAdapter{elements: []importer.ImportConfigAdapterElement{stubImportAdapter{}}}
		if got := netconfScopedImportAdapter(imp); got == nil {
			t.Fatal("want non-nil importer when elements present")
		}
	})

	t.Run("empty XML root becomes nil", func(t *testing.T) {
		doc := etree.NewDocument()
		root := doc.CreateElement("config")
		xmlImp := xmlimporter.NewXmlTreeImporter(root, consts.RunningIntentName, consts.RunningValuesPrio, false)
		if got := netconfScopedImportAdapter(xmlImp); got != nil {
			t.Error("want nil importer for empty XML tree")
		}
	})
}
