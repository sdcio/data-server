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

package datastore

import (
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/sdcio/data-server/pkg/tree/api"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

// canonicalLeafPath is the module-prefix-stripped, sorted-key path identifier
// shared across tree lookups and southbound readbacks.
type canonicalLeafPath string

// keylessLeafPath is the module-prefix-stripped, key-pruned XPath identifier
// for a leaf's schema path across all list instances.
type keylessLeafPath string

type captureMode uint8

const (
	captureConfirmed captureMode = iota
	capturePendingMutateOnWrite
	capturePendingCryptDiscovery
)

type confirmedAppliedEntry struct {
	path             *sdcpb.Path
	owner            string
	intendedValue    *sdcpb.TypedValue
	confirmedValue   *sdcpb.TypedValue
	pendingCaptureAt int64
	mode             captureMode
}

// causalCaptureEpoch is a snapshot of one mutate-on-write write epoch. Completed
// epochs retain confirmedValue so older in-flight syncs can be ignored.
type causalCaptureEpoch struct {
	path             *sdcpb.Path
	owner            string
	intendedValue    *sdcpb.TypedValue
	confirmedValue   *sdcpb.TypedValue
	captureAfterNano int64
	mode             captureMode
}

func (e causalCaptureEpoch) IsPending() bool {
	return e.mode != captureConfirmed
}

// confirmedAppliedValues holds the datastore's in-memory drift baselines and
// dynamically learned keyless schema paths for mutate-on-write leaves.
// Entries live strictly in process memory and are never written to cache storage.
type confirmedAppliedValues struct {
	mu           sync.RWMutex
	entries      map[canonicalLeafPath]confirmedAppliedEntry
	learnedPaths map[keylessLeafPath]struct{}
}

func newConfirmedAppliedValues() *confirmedAppliedValues {
	return &confirmedAppliedValues{
		entries:      make(map[canonicalLeafPath]confirmedAppliedEntry),
		learnedPaths: make(map[keylessLeafPath]struct{}),
	}
}

// Lookup returns a copy of the confirmed applied value only when path, winningOwner,
// and intendedValue all match the stored baseline.
func (s *confirmedAppliedValues) Lookup(path *sdcpb.Path, winningOwner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue {
	if s == nil || path == nil || winningOwner == "" || intendedValue == nil {
		return nil
	}
	key := newCanonicalLeafPath(path)

	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok || entry.confirmedValue == nil {
		return nil
	}
	if entry.owner != winningOwner || !api.TypedValuesEqual(entry.intendedValue, intendedValue) {
		return nil
	}
	return proto.Clone(entry.confirmedValue).(*sdcpb.TypedValue)
}

// Record stores confirmedValue as the baseline for (path, owner, intendedValue)
// when confirmedValue is non-empty. If intendedValue already matches the
// crypt/cipher allowlist and confirmedValue differs, any baseline for path is evicted.
func (s *confirmedAppliedValues) Record(path *sdcpb.Path, owner string, intendedValue, confirmedValue *sdcpb.TypedValue) {
	s.record(path, owner, intendedValue, confirmedValue, 0)
}

// RecordAfter stores a baseline and the write completion barrier that older
// in-flight sync snapshots must not cross, or evicts any baseline for path when
// a pre-hashed intendedValue differs from confirmedValue.
func (s *confirmedAppliedValues) RecordAfter(path *sdcpb.Path, owner string, intendedValue, confirmedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.record(path, owner, intendedValue, confirmedValue, captureAfterNano)
}

func (s *confirmedAppliedValues) record(path *sdcpb.Path, owner string, intendedValue, confirmedValue *sdcpb.TypedValue, captureAfterNano int64) {
	if s == nil || path == nil || owner == "" || intendedValue == nil || !isNonEmptyTypedValue(confirmedValue) {
		return
	}
	if rejectsPreHashedMismatch(intendedValue, confirmedValue) {
		s.Delete(path)
		return
	}
	key := newCanonicalLeafPath(path)

	s.mu.Lock()
	entry := confirmedAppliedEntry{
		path:             proto.Clone(path).(*sdcpb.Path),
		owner:            owner,
		intendedValue:    proto.Clone(intendedValue).(*sdcpb.TypedValue),
		confirmedValue:   proto.Clone(confirmedValue).(*sdcpb.TypedValue),
		pendingCaptureAt: captureAfterNano,
		mode:             captureConfirmed,
	}
	s.entries[key] = entry
	s.mu.Unlock()
}

// MarkPending retains the winning intent identity until a causally subsequent
// sync can supply the confirmed target value.
func (s *confirmedAppliedValues) MarkPending(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.markPending(path, owner, intendedValue, captureAfterNano, capturePendingMutateOnWrite)
}

// MarkDiscoveryCandidate records an unconfirmed push marker for an unannotated
// plain string leaf whose first causal sync may reveal a crypt or cipher value.
func (s *confirmedAppliedValues) MarkDiscoveryCandidate(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.markPending(path, owner, intendedValue, captureAfterNano, capturePendingCryptDiscovery)
}

func (s *confirmedAppliedValues) markPending(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue, captureAfterNano int64, mode captureMode) {
	if s == nil || path == nil || owner == "" || intendedValue == nil || captureAfterNano <= 0 {
		return
	}
	key := newCanonicalLeafPath(path)
	s.mu.Lock()
	s.entries[key] = confirmedAppliedEntry{
		path:             proto.Clone(path).(*sdcpb.Path),
		owner:            owner,
		intendedValue:    proto.Clone(intendedValue).(*sdcpb.TypedValue),
		pendingCaptureAt: captureAfterNano,
		mode:             mode,
	}
	s.mu.Unlock()
}

// CausalCaptures returns write epochs that still guard against older syncs.
// Pending entries also need the first causally subsequent observation.
func (s *confirmedAppliedValues) CausalCaptures() []causalCaptureEpoch {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]causalCaptureEpoch, 0)
	for _, entry := range s.entries {
		if entry.pendingCaptureAt == 0 {
			continue
		}
		result = append(result, causalCaptureEpoch{
			path:             proto.Clone(entry.path).(*sdcpb.Path),
			owner:            entry.owner,
			intendedValue:    proto.Clone(entry.intendedValue).(*sdcpb.TypedValue),
			captureAfterNano: entry.pendingCaptureAt,
			mode:             entry.mode,
		})
		if entry.confirmedValue != nil {
			result[len(result)-1].confirmedValue = proto.Clone(entry.confirmedValue).(*sdcpb.TypedValue)
		}
	}
	return result
}

// CompletePending resolves pending against the first causally subsequent
// observation: crypt-discovery epochs bind matching crypt/cipher observations
// and permanently learn the leaf's keyless schema path, mutate-on-write epochs
// bind non-empty observations unless a pre-hashed intendedValue differs, and
// non-matching or missing observations evict the epoch.
func (s *confirmedAppliedValues) CompletePending(pending causalCaptureEpoch, observed *sdcpb.TypedValue) {
	if s == nil || !pending.IsPending() {
		return
	}
	key := newCanonicalLeafPath(pending.path)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || entry.mode != pending.mode || entry.pendingCaptureAt != pending.captureAfterNano ||
		entry.owner != pending.owner || !api.TypedValuesEqual(entry.intendedValue, pending.intendedValue) {
		return
	}
	if pending.mode == capturePendingCryptDiscovery {
		if !isCryptDiscoveryMatch(pending.intendedValue, observed) {
			delete(s.entries, key)
			return
		}
		entry.confirmedValue = proto.Clone(observed).(*sdcpb.TypedValue)
		entry.mode = captureConfirmed
		s.entries[key] = entry
		if kp := newKeylessLeafPath(pending.path); kp != "" {
			s.learnedPaths[kp] = struct{}{}
		}
		return
	}
	if !isNonEmptyTypedValue(observed) || rejectsPreHashedMismatch(pending.intendedValue, observed) {
		delete(s.entries, key)
		return
	}
	entry.confirmedValue = proto.Clone(observed).(*sdcpb.TypedValue)
	entry.mode = captureConfirmed
	s.entries[key] = entry
}

// DeleteSubtree removes stored baselines or pending markers at or below prefix.
func (s *confirmedAppliedValues) DeleteSubtree(prefix *sdcpb.Path) {
	if s == nil || prefix == nil {
		return
	}
	prefixSlices := []*sdcpb.Path{prefix}
	s.mu.Lock()
	for key, entry := range s.entries {
		if pathCoveredBySync(entry.path, prefixSlices) {
			delete(s.entries, key)
		}
	}
	s.mu.Unlock()
}

// Delete removes any stored baseline for path.
func (s *confirmedAppliedValues) Delete(path *sdcpb.Path) {
	if s == nil || path == nil {
		return
	}
	key := newCanonicalLeafPath(path)
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

// ClearAll removes all stored baselines.
func (s *confirmedAppliedValues) ClearAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clear(s.entries)
	s.mu.Unlock()
}

var (
	_ api.ConfirmedAppliedValueResolver = (*confirmedAppliedValues)(nil)
	_ api.LearnedMutateOnWritePathSet   = (*confirmedAppliedValues)(nil)
)

// IsLearnedMutateOnWritePath reports whether path's keyless schema path was
// dynamically learned as a mutate-on-write leaf on this datastore.
func (s *confirmedAppliedValues) IsLearnedMutateOnWritePath(path *sdcpb.Path) bool {
	if s == nil {
		return false
	}
	kp := newKeylessLeafPath(path)
	if kp == "" {
		return false
	}
	s.mu.RLock()
	_, ok := s.learnedPaths[kp]
	s.mu.RUnlock()
	return ok
}

func newKeylessLeafPath(p *sdcpb.Path) keylessLeafPath {
	if p == nil || len(p.GetElem()) == 0 {
		return ""
	}
	return keylessLeafPath(canonicalPath(p).ToXPath(true))
}

func stripModulePrefix(name string) string {
	if _, local, ok := strings.Cut(name, ":"); ok {
		return local
	}
	return name
}

func canonicalPath(p *sdcpb.Path) *sdcpb.Path {
	if p == nil {
		return &sdcpb.Path{IsRootBased: true}
	}
	elems := make([]*sdcpb.PathElem, 0, len(p.GetElem()))
	for _, pe := range p.GetElem() {
		var keys map[string]string
		if len(pe.GetKey()) > 0 {
			keys = make(map[string]string, len(pe.GetKey()))
			for k, v := range pe.GetKey() {
				keys[stripModulePrefix(k)] = v
			}
		}
		elems = append(elems, sdcpb.NewPathElem(stripModulePrefix(pe.GetName()), keys))
	}
	return &sdcpb.Path{
		IsRootBased: true,
		Elem:        elems,
	}
}

var keyValEscaper = strings.NewReplacer(`\`, `\\`, `]`, `\]`)

func newCanonicalLeafPath(p *sdcpb.Path) canonicalLeafPath {
	if p == nil || len(p.GetElem()) == 0 {
		return "/"
	}
	var sb strings.Builder
	for _, pe := range p.GetElem() {
		sb.WriteByte('/')
		sb.WriteString(stripModulePrefix(pe.GetName()))
		if kv := pe.GetKey(); len(kv) > 0 {
			normalized := make(map[string]string, len(kv))
			keys := make([]string, 0, len(kv))
			for k, v := range kv {
				nk := stripModulePrefix(k)
				normalized[nk] = v
				keys = append(keys, nk)
			}
			slices.Sort(keys)
			for _, k := range keys {
				sb.WriteByte('[')
				sb.WriteString(k)
				sb.WriteByte('=')
				sb.WriteString(keyValEscaper.Replace(normalized[k]))
				sb.WriteByte(']')
			}
		}
	}
	return canonicalLeafPath(sb.String())
}

// isNonEmptyTypedValue reports whether tv is a non-empty scalar or leaf-list
// TypedValue suitable for a confirmed applied value. Unexpanded JSON envelopes
// are rejected so baselines always store schema-typed leaf values.
func isNonEmptyTypedValue(tv *sdcpb.TypedValue) bool {
	if tv == nil || tv.GetValue() == nil {
		return false
	}
	switch v := tv.GetValue().(type) {
	case *sdcpb.TypedValue_StringVal:
		return v.StringVal != ""
	case *sdcpb.TypedValue_AsciiVal:
		return v.AsciiVal != ""
	case *sdcpb.TypedValue_BytesVal:
		return len(v.BytesVal) > 0
	case *sdcpb.TypedValue_ProtoBytes:
		return len(v.ProtoBytes) > 0
	case *sdcpb.TypedValue_AnyVal:
		return v.AnyVal != nil && len(v.AnyVal.GetValue()) > 0
	case *sdcpb.TypedValue_JsonVal, *sdcpb.TypedValue_JsonIetfVal:
		return false
	case *sdcpb.TypedValue_LeaflistVal:
		if v.LeaflistVal == nil || len(v.LeaflistVal.GetElement()) == 0 {
			return false
		}
		for _, elem := range v.LeaflistVal.GetElement() {
			if !isNonEmptyTypedValue(elem) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

var dollarCryptPrefixes = []string{
	"$1$",
	"$2a$",
	"$2b$",
	"$2x$",
	"$2y$",
	"$5$",
	"$6$",
	"$7$",
	"$8$",
	"$9$",
	"$y$",
	"$gy$",
	"$ar2$",
	"$argon2i$",
	"$argon2id$",
	"$scrypt$",
	"$sha1$",
}

func containsWhitespace(s string) bool {
	return strings.IndexFunc(s, unicode.IsSpace) >= 0
}

// isCryptOrCipherString reports whether s matches the strict crypt or
// passphrase format allowlist used to identify target-hashed string secrets.
func isCryptOrCipherString(s string) bool {
	if s == "" {
		return false
	}
	for _, suffix := range []string{" hash2", " hash"} {
		if token, ok := strings.CutSuffix(s, suffix); ok {
			return token != "" && !containsWhitespace(token)
		}
	}
	if containsWhitespace(s) {
		return false
	}
	if strings.HasPrefix(s, "000PASSPHRASE000") {
		return true
	}
	for _, prefix := range dollarCryptPrefixes {
		if strings.HasPrefix(s, prefix) {
			return len(s) > len(prefix)
		}
	}
	if rest, ok := strings.CutPrefix(s, "$pbkdf2"); ok {
		if strings.HasPrefix(rest, "$") {
			return len(rest) > 1
		}
		if strings.HasPrefix(rest, "-") {
			alg, tail, hasDollar := strings.Cut(rest[1:], "$")
			return hasDollar && alg != "" && tail != ""
		}
	}
	return false
}

func matchesCryptOrCipherValue(tv *sdcpb.TypedValue) bool {
	sv, ok := tv.GetValue().(*sdcpb.TypedValue_StringVal)
	return ok && isCryptOrCipherString(sv.StringVal)
}

func rejectsPreHashedMismatch(intendedValue, observed *sdcpb.TypedValue) bool {
	return matchesCryptOrCipherValue(intendedValue) && !api.TypedValuesEqual(observed, intendedValue)
}

func isCryptDiscoveryMatch(intendedValue, observed *sdcpb.TypedValue) bool {
	if !isNonEmptyTypedValue(observed) || api.TypedValuesEqual(observed, intendedValue) {
		return false
	}
	return !matchesCryptOrCipherValue(intendedValue) && matchesCryptOrCipherValue(observed)
}
