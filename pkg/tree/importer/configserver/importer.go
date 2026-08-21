// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package configserver

import (
	"fmt"

	csreader "github.com/sdcio/data-server/pkg/cache/configserver"
	"github.com/sdcio/data-server/pkg/tree/importer"
	jsonimporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// documentImporter adapts a Document — the local-read seam's representation
// of a config-server Config (joined with its SensitiveConfig) — into an
// importer.ImportConfigAdapter, per the ADR's field-mapping table. Traversal
// of the merged config payload is delegated to
// importer/json.JsonTreeImporter, which already knows how to walk the
// JSON_IETF-shaped map mergeConfigBlobs produces; only the two accessors
// JsonTreeImporter always stubs out for its own (synced-device-data) use
// case are overridden here, from Document fields the ADR maps them to.
type documentImporter struct {
	*jsonimporter.JsonTreeImporter
	orphan         bool
	sensitivePaths []*sdcpb.Path
}

// NewImportAdapter builds the importer.ImportConfigAdapter for doc.
func NewImportAdapter(doc *csreader.Document) (importer.ImportConfigAdapter, error) {
	root, err := mergeConfigBlobs(doc.Config)
	if err != nil {
		return nil, fmt.Errorf("configserver: building config for %q: %w", doc.Name, err)
	}
	return &documentImporter{
		JsonTreeImporter: jsonimporter.NewJsonTreeImporter(root, doc.IntentName(), doc.Priority, doc.NonRevertive),
		orphan:           doc.Orphan,
		sensitivePaths:   doc.SensitivePaths,
	}, nil
}

func (d *documentImporter) GetOrphan() bool {
	return d.orphan
}

func (d *documentImporter) GetSensitivePaths() []*sdcpb.Path {
	return d.sensitivePaths
}

var _ importer.ImportConfigAdapter = (*documentImporter)(nil)
