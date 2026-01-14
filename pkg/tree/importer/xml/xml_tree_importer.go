package xml

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type XmlTreeImporter struct {
	elem *etree.Element
}

func NewXmlTreeImporter(d *etree.Element) *XmlTreeImporter {
	return &XmlTreeImporter{
		elem: d,
	}
}

func (x *XmlTreeImporter) GetDeletes() *sdcpb.PathSet {
	return sdcpb.NewPathSet()
}

func (x *XmlTreeImporter) GetElement(key string) importer.ImportConfigAdapterElement {
	e := x.elem.FindElement(key)
	if e == nil {
		return nil
	}
	return NewXmlTreeImporter(e)
}

func (x *XmlTreeImporter) GetElements() []importer.ImportConfigAdapterElement {
	childs := x.elem.ChildElements()
	if len(childs) == 0 {
		return nil
	}

	result := make([]importer.ImportConfigAdapterElement, 0, len(childs))

	for _, c := range childs {
		result = append(result, NewXmlTreeImporter(c))
	}

	return result
}

func (x *XmlTreeImporter) GetKeyValue() (string, error) {
	return x.elem.Text(), nil
}

func (x *XmlTreeImporter) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return utils.Convert(ctx, x.elem.Text(), slt)
}

func (x *XmlTreeImporter) GetName() string {
	return x.elem.Tag
}

// Function to ensure JsonTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*XmlTreeImporter)(nil)
