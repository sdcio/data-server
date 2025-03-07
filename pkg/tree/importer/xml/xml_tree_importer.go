package xml

import (
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

func (x *XmlTreeImporter) GetElement(key string) importer.ImportConfigAdapter {
	e := x.elem.FindElement(key)
	if e == nil {
		return nil
	}
	return NewXmlTreeImporter(e)
}

func (x *XmlTreeImporter) GetElements() []importer.ImportConfigAdapter {
	childs := x.elem.ChildElements()
	if len(childs) == 0 {
		return nil
	}

	result := make([]importer.ImportConfigAdapter, 0, len(childs))

	for _, c := range childs {
		result = append(result, NewXmlTreeImporter(c))
	}

	return result
}

func (x *XmlTreeImporter) GetKeyValue() string {
	return x.elem.Text()
}

func (x *XmlTreeImporter) GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return utils.Convert(x.elem.Text(), slt)
}

func (x *XmlTreeImporter) GetName() string {
	return x.elem.Tag
}

// Function to ensure JsonTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*XmlTreeImporter)(nil)
