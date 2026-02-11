package xml

import (
	"context"

	"github.com/beevik/etree"
	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type XmlTreeImporter struct {
	*XmlTreeImporterElement
	intentName   string
	priority     int32
	nonRevertive bool
}

func NewXmlTreeImporter(d *etree.Element, intentName string, priority int32, nonRevertive bool) *XmlTreeImporter {
	return &XmlTreeImporter{
		XmlTreeImporterElement: NewXmlTreeImporterElement(d),
		intentName:             intentName,
		priority:               priority,
		nonRevertive:           nonRevertive,
	}
}

func (x *XmlTreeImporter) GetName() string {
	return x.intentName
}

func (x *XmlTreeImporter) GetPriority() int32 {
	return x.priority
}

func (x *XmlTreeImporter) GetNonRevertive() bool {
	return x.nonRevertive
}

type XmlTreeImporterElement struct {
	elem *etree.Element
}

func NewXmlTreeImporterElement(d *etree.Element) *XmlTreeImporterElement {
	return &XmlTreeImporterElement{
		elem: d,
	}
}

func (x *XmlTreeImporterElement) GetDeletes() *sdcpb.PathSet {
	return sdcpb.NewPathSet()
}

func (x *XmlTreeImporterElement) GetElement(key string) importer.ImportConfigAdapterElement {
	e := x.elem.FindElement(key)
	if e == nil {
		return nil
	}
	return NewXmlTreeImporterElement(e)
}

func (x *XmlTreeImporterElement) GetElements() []importer.ImportConfigAdapterElement {
	childs := x.elem.ChildElements()
	if len(childs) == 0 {
		return nil
	}

	result := make([]importer.ImportConfigAdapterElement, 0, len(childs))

	for _, c := range childs {
		result = append(result, NewXmlTreeImporterElement(c))
	}

	return result
}

func (x *XmlTreeImporterElement) GetKeyValue() (string, error) {
	return x.elem.Text(), nil
}

func (x *XmlTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return sdcpb.TVFromString(slt, x.elem.Text(), 0)
}

func (x *XmlTreeImporterElement) GetName() string {
	return x.elem.Tag
}

// Function to ensure JsonTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*XmlTreeImporter)(nil)
var _ importer.ImportConfigAdapterElement = (*XmlTreeImporterElement)(nil)
