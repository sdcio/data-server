package json

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type JsonTreeImporter struct {
	data any
	name string
}

func newJsonTreeImporterInternal(name string, d any) *JsonTreeImporter {
	return &JsonTreeImporter{
		data: d,
		name: name,
	}
}

func NewJsonTreeImporter(d any) *JsonTreeImporter {
	return &JsonTreeImporter{
		data: d,
		name: "root",
	}
}

func (j *JsonTreeImporter) GetElement(key string) importer.ImportConfigAdapter {
	switch d := j.data.(type) {
	case map[string]any:
		return newJsonTreeImporterInternal(key, d[key])
	}
	return nil
}

func (j *JsonTreeImporter) GetElements() []importer.ImportConfigAdapter {
	var result []importer.ImportConfigAdapter
	switch d := j.data.(type) {
	case map[string]any:
		result = make([]importer.ImportConfigAdapter, 0, len(d))
		for k, v := range d {
			switch subElem := v.(type) {
			case []any:
				for _, listElem := range subElem {
					result = append(result, newJsonTreeImporterInternal(k, listElem))
				}
			default:
				result = append(result, newJsonTreeImporterInternal(k, v))
			}
		}
	default:
		fmt.Println("SHOULD NOT HAPPEN!")
	}
	return result
}

func (j *JsonTreeImporter) GetValue() string {
	return fmt.Sprintf("%v", j.data)
}

func (j *JsonTreeImporter) GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return utils.ConvertJsonValueToTv(j.data, slt)
}

func (j *JsonTreeImporter) GetName() string {
	return j.name
}
