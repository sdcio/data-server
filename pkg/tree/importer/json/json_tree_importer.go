package json

import (
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/importer"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
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

		for k, v := range d {
			beforeColon, elemName, found := strings.Cut(k, ":")
			if !found {
				elemName = beforeColon
			}
			if key == elemName {
				return newJsonTreeImporterInternal(key, v)
			}
		}
	}
	return nil
}

func (j *JsonTreeImporter) GetElements() []importer.ImportConfigAdapter {
	var result []importer.ImportConfigAdapter
	switch d := j.data.(type) {
	case map[string]any:
		result = make([]importer.ImportConfigAdapter, 0, len(d))
		for k, v := range d {
			beforeColon, key, found := strings.Cut(k, ":")
			if !found {
				key = beforeColon
			}
			switch subElem := v.(type) {
			case []any:
				for _, listElem := range subElem {
					result = append(result, newJsonTreeImporterInternal(key, listElem))
				}
			default:
				result = append(result, newJsonTreeImporterInternal(key, v))
			}
		}
	default:
		log.Error("error we hit a code path that was not meant to be hit.")
	}
	return result
}

func (j *JsonTreeImporter) GetKeyValue() (string, error) {
	return fmt.Sprintf("%v", j.data), nil
}

func (j *JsonTreeImporter) GetTVValue(slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return utils.ConvertJsonValueToTv(j.data, slt)
}

func (j *JsonTreeImporter) GetName() string {
	return j.name
}

// Function to ensure JsonTreeImporter implements ImportConfigAdapter (optional)
var _ importer.ImportConfigAdapter = (*JsonTreeImporter)(nil)
