package proto

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/tree/tree_persist"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

type ProtoTreeImporter struct {
	ProtoTreeImporterElement
	deletes *sdcpb.PathSet
}

func NewProtoTreeImporter(data *tree_persist.TreeElement) *ProtoTreeImporter {
	return &ProtoTreeImporter{
		ProtoTreeImporterElement: ProtoTreeImporterElement{
			data: data,
		},
	}
}

type ProtoTreeImporterElement struct {
	data *tree_persist.TreeElement
}

func NewProtoTreeImporterElement(data *tree_persist.TreeElement) *ProtoTreeImporterElement {
	return &ProtoTreeImporterElement{
		data: data,
	}
}

func (p *ProtoTreeImporter) GetDeletes() *sdcpb.PathSet {
	return p.deletes
}

func (p *ProtoTreeImporterElement) GetElements() []importer.ImportConfigAdapterElement {
	if len(p.data.Childs) == 0 {
		return nil
	}
	result := []importer.ImportConfigAdapterElement{}
	for _, c := range p.data.Childs {
		result = append(result, NewProtoTreeImporterElement(c))
	}
	return result
}

func (p *ProtoTreeImporterElement) GetElement(key string) importer.ImportConfigAdapterElement {
	for _, c := range p.data.Childs {
		if c.Name == key {
			return NewProtoTreeImporterElement(c)
		}
	}
	return nil
}

func (p *ProtoTreeImporterElement) GetKeyValue() (string, error) {
	tv, err := p.GetTVValue(nil)
	if err != nil {
		return "", fmt.Errorf("failed GetTVValue for %s", p.data.Name)
	}
	return utils.TypedValueToString(tv), nil
}

func (p *ProtoTreeImporterElement) GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	result := &sdcpb.TypedValue{}
	err := proto.Unmarshal(p.data.LeafVariant, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (p *ProtoTreeImporterElement) GetName() string {
	return p.data.Name
}

// Function to ensure ProtoTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*ProtoTreeImporter)(nil)
