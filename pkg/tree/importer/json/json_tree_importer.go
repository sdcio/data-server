package json

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/importer"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type JsonTreeImporter struct {
	*JsonTreeImporterElement
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
		JsonTreeImporterElement: newJsonTreeImporterElement("root", d),
		intentName:              intentName,
		priority:                priority,
		nonRevertive:            nonRevertive,
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

// GetElement returns a child element by key, or nil if not found.
// Tries exact match first, then falls back to local-name match (after ":") to handle
// RFC 7951 JSON_IETF module-prefixed keys (e.g. "openconfig-if:name" matched by "name").
func (j *JsonTreeImporterElement) GetElement(key string) importer.ImportConfigAdapterElement {
	switch d := j.data.(type) {
	case map[string]any:
		// Exact match first.
		if v, ok := d[key]; ok {
			logf.DefaultLogger.V(logf.VTrace).Info("traversing element", "element", key)
			return newJsonTreeImporterElement(key, v)
		}
		// Local-name fallback: find data key whose local part (after ":") matches.
		for k, v := range d {
			_, localName, found := strings.Cut(k, ":")
			if found && localName == key {
				logf.DefaultLogger.V(logf.VTrace).Info("traversing element by local-name", "element", key, "dataKey", k)
				return newJsonTreeImporterElement(key, v)
			}
		}
	}
	return nil
}

// GetElements returns all child elements at this level.
// Module prefixes in keys are stripped to local names so the processor can look them up
// in the schema tree by bare name. Plain JSON keys (no ":") are passed through unchanged.
func (j *JsonTreeImporterElement) GetElements() []importer.ImportConfigAdapterElement {
	var result []importer.ImportConfigAdapterElement
	switch d := j.data.(type) {
	case map[string]any:
		result = make([]importer.ImportConfigAdapterElement, 0, len(d))
		for k, v := range d {
			name := k
			if _, localName, found := strings.Cut(k, ":"); found {
				name = localName
			}
			logf.DefaultLogger.V(logf.VTrace).Info("traversing element", "element", name, "dataKey", k)
			switch subElem := v.(type) {
			case []any:
				for _, listElem := range subElem {
					result = append(result, newJsonTreeImporterElement(name, listElem))
				}
			default:
				result = append(result, newJsonTreeImporterElement(name, v))
			}
		}
	default:
		logf.DefaultLogger.Error(nil, "hit a code path that was not meant to be hit")
	}
	return result
}

func (j *JsonTreeImporterElement) GetKeyValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (string, error) {
	return fmt.Sprintf("%v", j.data), nil
}

func (j *JsonTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, *sdcpb.SchemaLeafType, error) {
	return sdcpb.ConvertJsonValueToTvWithType(j.data, slt)
}

func (j *JsonTreeImporterElement) GetName() string {
	return j.name
}

var _ importer.ImportConfigAdapter = (*JsonTreeImporter)(nil)
var _ importer.ImportConfigAdapterElement = (*JsonTreeImporterElement)(nil)
