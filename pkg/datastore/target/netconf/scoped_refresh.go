package netconf

import (
	"github.com/sdcio/data-server/pkg/tree/importer"
)

// netconfScopedImportAdapter maps an empty device snapshot to a nil importer so
// ApplyToRunning performs path-scoped refresh under configured sync paths, matching
// gNMI GET empty-cycle semantics.
func netconfScopedImportAdapter(imp importer.ImportConfigAdapter) importer.ImportConfigAdapter {
	if imp == nil {
		return nil
	}
	if ps := imp.GetDeletes(); ps != nil && len(ps.ToPathSlice()) > 0 {
		return imp
	}
	if len(imp.GetElements()) > 0 {
		return imp
	}
	return nil
}
