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
	data *tree_persist.TreeElement
}

func NewProtoTreeImporter(data *tree_persist.TreeElement) *ProtoTreeImporter {
	return &ProtoTreeImporter{
		data: data,
	}
}

func (p *ProtoTreeImporter) GetElements() []importer.ImportConfigAdapter {
	if len(p.data.Childs) == 0 {
		return nil
	}
	result := []importer.ImportConfigAdapter{}
	for _, c := range p.data.Childs {
		result = append(result, NewProtoTreeImporter(c))
	}
	return result
}

// func (p *ProtoTreeImporter) getListElements(elems []*tree_persist.TreeElement) []*tree_persist.TreeElement {
// 	result := make([]*tree_persist.TreeElement, 0, len(elems))
// 	for _, elem := range elems {
// 		if elem.Name == "" {
// 			result = append(result, &tree_persist.TreeElement{Name: p.data.Name, Childs: p.getListElements(elem.GetChilds())})
// 		} else {
// 			result = append(result, p.getListElements(elem.GetChilds())...)
// 		}
// 	}
// 	return result
// }

func (p *ProtoTreeImporter) GetElement(key string) importer.ImportConfigAdapter {
	for _, c := range p.data.Childs {
		if c.Name == key {
			return NewProtoTreeImporter(c)
		}
	}
	return nil
}

func (p *ProtoTreeImporter) GetKeyValue() (string, error) {
	tv, err := p.GetTVValue(nil)
	if err != nil {
		return "", fmt.Errorf("failed GetTVValue for %s", p.data.Name)
	}
	return utils.TypedValueToString(tv), nil
}

func (p *ProtoTreeImporter) GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	result := &sdcpb.TypedValue{}
	err := proto.Unmarshal(p.data.LeafVariant, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (p *ProtoTreeImporter) GetName() string {
	return p.data.Name
}

// Function to ensure ProtoTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*ProtoTreeImporter)(nil)
