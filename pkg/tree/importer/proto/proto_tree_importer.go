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
	deletes      *sdcpb.PathSet
	intentName   string
	priority     int32
	nonRevertive bool
}

func NewProtoTreeImporter(data *tree_persist.Intent) *ProtoTreeImporter {
	pathSet := sdcpb.NewPathSet()
	pathSet.AddPaths(data.GetExplicitDeletes())

	return &ProtoTreeImporter{
		ProtoTreeImporterElement: ProtoTreeImporterElement{
			data: data.GetRoot(),
		},
		deletes:      pathSet,
		intentName:   data.GetIntentName(),
		priority:     data.GetPriority(),
		nonRevertive: data.GetNonRevertive(),
	}
}

func (p *ProtoTreeImporter) GetPriority() int32 {
	return p.priority
}

func (p *ProtoTreeImporter) GetNonRevertive() bool {
	return p.nonRevertive
}

func (p *ProtoTreeImporter) GetName() string {
	return p.intentName
}

func (p *ProtoTreeImporter) GetDeletes() *sdcpb.PathSet {
	return p.deletes
}

type ProtoTreeImporterElement struct {
	data *tree_persist.TreeElement
}

func NewProtoTreeImporterElement(data *tree_persist.TreeElement) *ProtoTreeImporterElement {
	return &ProtoTreeImporterElement{
		data: data,
	}
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

func (p *ProtoTreeImporterElement) GetKeyValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (string, error) {
	tv, _, err := p.GetTVValue(ctx, slt)
	if err != nil {
		return "", fmt.Errorf("failed GetTVValue for %s", p.data.Name)
	}
	return tv.ToString(), nil
}

// GetTVValue unmarshals the proto-serialized TypedValue. For union-typed leaves,
// InferUnionMemberFromTypedValue attaches the matched branch per RFC 7950 §9.12
// (first matching member in schema order; same narrowing as TVFromStringWithType).
func (p *ProtoTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, *sdcpb.SchemaLeafType, error) {
	result := &sdcpb.TypedValue{}
	err := proto.Unmarshal(p.data.LeafVariant, result)
	if err != nil {
		return nil, nil, err
	}
	matched := importer.InferUnionMemberFromTypedValue(result, slt)
	return result, matched, nil
}

func (p *ProtoTreeImporterElement) GetName() string {
	return p.data.Name
}

// Function to ensure ProtoTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*ProtoTreeImporter)(nil)
var _ importer.ImportConfigAdapterElement = (*ProtoTreeImporterElement)(nil)
