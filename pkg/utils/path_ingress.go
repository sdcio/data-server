package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

// PathForSchemaLookup returns a path suitable for schema-server GetSchema:
// module:local on the first element becomes Path.Origin plus a bare local name.
func PathForSchemaLookup(p *sdcpb.Path) *sdcpb.Path {
	if p == nil {
		return nil
	}
	lookup := proto.Clone(p).(*sdcpb.Path)
	if len(lookup.GetElem()) == 0 {
		return lookup
	}
	first := lookup.GetElem()[0]
	if mod, local, ok := strings.Cut(first.GetName(), ":"); ok && mod != "" && local != "" {
		if lookup.GetOrigin() == "" {
			lookup.Origin = mod
		}
		first.Name = local
	}
	return lookup
}

func decodeUpdateJSON(upd *sdcpb.Update) (any, error) {
	if upd.GetValue() == nil {
		return nil, fmt.Errorf("decodeUpdateJSON: value is nil")
	}
	var raw []byte
	switch upd.GetValue().Value.(type) {
	case *sdcpb.TypedValue_JsonIetfVal:
		raw = upd.GetValue().GetJsonIetfVal()
	case *sdcpb.TypedValue_JsonVal:
		raw = upd.GetValue().GetJsonVal()
	default:
		return nil, fmt.Errorf("decodeUpdateJSON: not a JSON typed value")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// anchorRootModuleFromJSONIETF sets Path.Origin (and path elems when empty) from
// RFC 7951 module-qualified top-level JSON keys when the path does not already
// carry a root module. Does not use sync config or other out-of-band hints.
func anchorRootModuleFromJSONIETF(path *sdcpb.Path, jv map[string]any) error {
	if path == nil || path.GetOrigin() != "" {
		return nil
	}
	wantLocal := ""
	if len(path.GetElem()) > 0 {
		wantLocal = path.GetElem()[0].GetName()
		if mod, local, ok := strings.Cut(wantLocal, ":"); ok && mod != "" && local != "" {
			path.Origin = mod
			path.Elem[0].Name = local
			return nil
		}
	}
	type candidate struct {
		module string
		local  string
	}
	var matches []candidate
	for k := range jv {
		if k == "_annotate" {
			continue
		}
		id := parseJSONIETFKey(k)
		if id.module == "" {
			continue
		}
		if wantLocal != "" && id.local != wantLocal {
			continue
		}
		matches = append(matches, candidate{module: id.module, local: id.local})
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 1 {
		mods := make([]string, len(matches))
		for i, m := range matches {
			mods[i] = m.module
		}
		return fmt.Errorf("ambiguous root module from JSON-IETF keys for local %q: %v", wantLocal, mods)
	}
	path.Origin = matches[0].module
	if wantLocal == "" {
		path.Elem = []*sdcpb.PathElem{{Name: matches[0].local}}
		path.IsRootBased = true
	}
	return nil
}

// LogIngressExpandFailure records path and JSON-IETF top-level keys when schema
// lookup fails at expand time (e.g. ambiguous colliding roots).
func LogIngressExpandFailure(ctx context.Context, upd *sdcpb.Update, jsonDecoded any, err error) {
	log := logger.FromContext(ctx)
	if !log.V(logger.VDebug).Enabled() {
		return
	}
	p := upd.GetPath()
	elems := make([]string, 0, len(p.GetElem()))
	for _, e := range p.GetElem() {
		elems = append(elems, e.GetName())
	}
	keys := topLevelJSONKeys(jsonDecoded)
	log.Info("ingress expand failed",
		"path-origin", p.GetOrigin(),
		"path-elems", elems,
		"json-top-level-keys", keys,
		"err", err,
	)
}

func topLevelJSONKeys(v any) []string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "_annotate" {
			keys = append(keys, k)
		}
	}
	return keys
}
