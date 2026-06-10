package types

import (
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// RedactedStringValue is the sentinel substituted in place of sensitive leaf
// values in deviation, render, and blame outputs.
const RedactedStringValue = "***"

// RedactedTypedValue is the canonical TypedValue emitted in place of a
// sensitive leaf. Callers must not mutate the returned pointer.
var RedactedTypedValue = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: RedactedStringValue}}

// SensitivePathChecker is the read-only interface consumed by tree-layer
// render and deviation operations. Both *SensitivePathIndex (snapshot mode via
// Add) and *SensitivePathIndex (live mode via Set/Delete) satisfy it.
type SensitivePathChecker interface {
	Contains(path *sdcpb.Path) bool
}

// ShouldRedact reports whether a leaf value must be replaced with the
// redaction sentinel. It is the single authoritative implementation of the
// two-source sensitivity check; both pkg/tree/ops (entry-aware wrapper) and
// pkg/tree/api (leaf_variants.go) delegate here.
//
// Parameters:
//   - includeSensitive: caller has opted in to receiving real values — skip redaction.
//   - schema:           the entry's schema element (pass entry.GetSchema()); nil is safe.
//   - path:             the entry's sdcpb path, used for intent path-marker lookup.
//   - sps:              cross-intent path-marker set; nil means no path-marker redaction.
func ShouldRedact(includeSensitive bool, schema *sdcpb.SchemaElem, path *sdcpb.Path, sps SensitivePathChecker) bool {
	if includeSensitive {
		return false
	}
	return (schema != nil && schema.IsSensitive()) || (sps != nil && sps.Contains(path))
}

var _ SensitivePathChecker = (*SensitivePathIndex)(nil)

// SensitivePathIndex is a concurrent-safe index of sensitive schema paths.
//
// It supports two usage modes that share one struct:
//
//   - Live index (datastore-wide): use Set/Delete for incremental per-intent
//     updates. The map value tracks which intents own each path so that Delete
//     can surgically remove only that intent's contribution.
//
//   - Snapshot (per-intent): use Add to mark presence without intent tracking.
//     The map value is nil — key existence is the only signal Contains needs.
//
// The two modes must not be mixed on the same instance.
type SensitivePathIndex struct {
	mu    sync.RWMutex
	index map[string][]string // key-pruned XPath → intent names (nil = presence-only)
}

// NewSensitivePathIndex returns a ready-to-use, empty SensitivePathIndex.
func NewSensitivePathIndex() *SensitivePathIndex {
	return &SensitivePathIndex{index: make(map[string][]string)}
}

// Add marks each path as sensitive without associating it with an intent name
// (presence-only mode). Safe to call concurrently.
func (s *SensitivePathIndex) Add(paths ...*sdcpb.Path) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range paths {
		if p != nil {
			s.index[p.ToXPath(true)] = nil
		}
	}
}

// Set replaces the sensitive paths contributed by intentName with paths.
// Existing entries for intentName are removed before the new ones are added.
// Safe to call concurrently.
func (s *SensitivePathIndex) Set(intentName string, paths []*sdcpb.Path) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(intentName)
	for _, p := range paths {
		if p == nil {
			continue
		}
		key := p.ToXPath(true)
		s.index[key] = append(s.index[key], intentName)
	}
}

// Delete removes all sensitive-path contributions from intentName.
// Safe to call concurrently.
func (s *SensitivePathIndex) Delete(intentName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(intentName)
}

// removeLocked removes intentName from every entry in the index.
// Entries whose slice becomes empty are deleted from the map.
// Caller must hold the write lock.
func (s *SensitivePathIndex) removeLocked(intentName string) {
	for key, names := range s.index {
		if names == nil {
			continue // presence-only entry (Add mode) — not managed by intent tracking
		}
		j := 0
		for _, n := range names {
			if n != intentName {
				names[j] = n
				j++
			}
		}
		if j == 0 {
			delete(s.index, key)
		} else {
			s.index[key] = names[:j]
		}
	}
}

// Contains reports whether path is in the index. The lookup uses the same
// key-pruned XPath form as the stored keys, so /interface[name=eth0]/secret
// matches a stored /interface/secret. Safe to call on a nil receiver.
func (s *SensitivePathIndex) Contains(path *sdcpb.Path) bool {
	if s == nil || path == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.index[path.ToXPath(true)]
	return ok
}
