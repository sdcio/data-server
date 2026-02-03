package json

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/utils"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type JsonTreeImporter struct {
	JsonTreeImporterElement
	intentName   string
	priority     int32
	nonRevertive bool
}

func (j *JsonTreeImporter) GetPriority() int32 {
	return j.priority
}

func (j *JsonTreeImporter) GetNonRevertive() bool {
	return j.nonRevertive
}

func (j *JsonTreeImporter) GetName() string {
	return j.intentName
}

func NewJsonTreeImporter(d any, intentName string, priority int32, nonRevertive bool) *JsonTreeImporter {
	return &JsonTreeImporter{
		JsonTreeImporterElement: JsonTreeImporterElement{
			data: d,
			name: "root",
		},
		intentName:   intentName,
		priority:     priority,
		nonRevertive: nonRevertive,
	}
}

type JsonTreeImporterElement struct {
	data any
	name string
}

func newJsonTreeImporterElement(name string, d any) *JsonTreeImporterElement {
	return &JsonTreeImporterElement{
		data: d,
		name: name,
	}
}

func (j *JsonTreeImporterElement) GetDeletes() *sdcpb.PathSet {
	return sdcpb.NewPathSet()
}

func (j *JsonTreeImporterElement) GetElement(key string) importer.ImportConfigAdapterElement {
	switch d := j.data.(type) {
	case map[string]any:

		for k, v := range d {
			beforeColon, elemName, found := strings.Cut(k, ":")
			if !found {
				elemName = beforeColon
			}
			if key == elemName {
				return newJsonTreeImporterElement(key, v)
			}
		}
	}
	return nil
}

func (j *JsonTreeImporterElement) GetElements() []importer.ImportConfigAdapterElement {
	var result []importer.ImportConfigAdapterElement
	switch d := j.data.(type) {
	case map[string]any:
		result = make([]importer.ImportConfigAdapterElement, 0, len(d))
		for k, v := range d {
			beforeColon, key, found := strings.Cut(k, ":")
			if !found {
				key = beforeColon
			}
			switch subElem := v.(type) {
			case []any:
				for _, listElem := range subElem {
					result = append(result, newJsonTreeImporterElement(key, listElem))
				}
			default:
				result = append(result, newJsonTreeImporterElement(key, v))
			}
		}
	default:
		logf.DefaultLogger.Error(nil, "hit a code path that was not meant to be hit")
	}
	return result
}

func (j *JsonTreeImporterElement) GetKeyValue() (string, error) {
	return fmt.Sprintf("%v", j.data), nil
}

func (j *JsonTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return utils.ConvertJsonValueToTv(j.data, slt)
}

func (j *JsonTreeImporterElement) GetName() string {
	return j.name
}

// Function to ensure JsonTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*JsonTreeImporter)(nil)
var _ importer.ImportConfigAdapterElement = (*JsonTreeImporterElement)(nil)
