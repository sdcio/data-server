package proto

import (
	"context"
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/importer"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/sdc-protos/tree_persist"
	"google.golang.org/protobuf/proto"
)

type ProtoTreeImporter struct {
	ProtoTreeImporterElement
	deletes *sdcpb.PathSet
}

func NewProtoTreeImporter(data *tree_persist.Intent) *ProtoTreeImporter {
	pathSet := sdcpb.NewPathSet()
	pathSet.AddPaths(data.GetExplicitDeletes())

	return &ProtoTreeImporter{
		ProtoTreeImporterElement: ProtoTreeImporterElement{
			data: data.GetRoot(),
		},
		deletes: pathSet,
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
	if p.data == nil || len(p.data.Childs) == 0 {
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
	tv, err := p.GetTVValue(context.Background(), nil)
	if err != nil {
		return "", fmt.Errorf("failed GetTVValue for %s", p.data.Name)
	}
	return tv.ToString(), nil
}

func (p *ProtoTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
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
