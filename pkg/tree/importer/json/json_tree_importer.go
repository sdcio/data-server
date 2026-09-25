package json

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
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
	data     any
	identity api.NodeIdentity
}

func newJsonTreeImporterElement(dataKey string, d any) *JsonTreeImporterElement {
	id := api.ParseJSONIETFKey(dataKey)
	if dataKey == "root" {
		id = api.LocalIdentity("root")
	}
	return &JsonTreeImporterElement{
		data:     d,
		identity: id,
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
				return newJsonTreeImporterElement(k, v)
			}
		}
	}
	return nil
}

// GetElements returns all child elements at this level.
// Module prefixes in JSON keys are preserved in Identity(); GetName() is the YANG local name.
func (j *JsonTreeImporterElement) GetElements() []importer.ImportConfigAdapterElement {
	var result []importer.ImportConfigAdapterElement
	switch d := j.data.(type) {
	case map[string]any:
		result = make([]importer.ImportConfigAdapterElement, 0, len(d))
		for k, v := range d {
			logf.DefaultLogger.V(logf.VTrace).Info("traversing element", "element", api.ParseJSONIETFKey(k).Local, "dataKey", k)
			switch subElem := v.(type) {
			case []any:
				for _, listElem := range subElem {
					result = append(result, newJsonTreeImporterElement(k, listElem))
				}
			default:
				result = append(result, newJsonTreeImporterElement(k, v))
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

func (j *JsonTreeImporterElement) GetTVValue(ctx context.Context, slt *sdcpb.SchemaLeafType) (*sdcpb.TypedValue, error) {
	return sdcpb.ConvertJsonValueToTv(j.data, slt)
}

func (j *JsonTreeImporterElement) GetName() string {
	return j.identity.Local
}

func (j *JsonTreeImporterElement) Identity() api.NodeIdentity {
	return j.identity
}

var _ importer.ImportConfigAdapter = (*JsonTreeImporter)(nil)
var _ importer.ImportConfigAdapterElement = (*JsonTreeImporterElement)(nil)
