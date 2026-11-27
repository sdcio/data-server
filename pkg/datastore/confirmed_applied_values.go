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
	"maps"
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

type baselineMode uint8

const (
	modeConfirmed baselineMode = iota
	modeMutateOnWrite
	modeCryptDiscovery
)

// baselineIdentity identifies the winning intent state to which a confirmed
// applied value or pending capture epoch is bound.
type baselineIdentity struct {
	path          *sdcpb.Path
	owner         string
	intendedValue *sdcpb.TypedValue
}

func cloneBaselineIdentity(id baselineIdentity) baselineIdentity {
	return baselineIdentity{
		path:          proto.Clone(id.path).(*sdcpb.Path),
		owner:         id.owner,
		intendedValue: proto.Clone(id.intendedValue).(*sdcpb.TypedValue),
	}
}

func (id baselineIdentity) matches(owner string, intendedValue *sdcpb.TypedValue) bool {
	return id.owner == owner && api.TypedValuesEqual(id.intendedValue, intendedValue)
}

type confirmedAppliedEntry struct {
	baselineIdentity
	confirmedValue          *sdcpb.TypedValue
	captureAfterNano        int64
	awaitingInitialSyncSeed bool
	mode                    baselineMode
}

func cloneConfirmedAppliedEntry(e confirmedAppliedEntry) confirmedAppliedEntry {
	out := confirmedAppliedEntry{
		baselineIdentity:        cloneBaselineIdentity(e.baselineIdentity),
		captureAfterNano:        e.captureAfterNano,
		awaitingInitialSyncSeed: e.awaitingInitialSyncSeed,
		mode:                    e.mode,
	}
	if e.confirmedValue != nil {
		out.confirmedValue = proto.Clone(e.confirmedValue).(*sdcpb.TypedValue)
	}
	return out
}

// causalCaptureEpoch is an immutable snapshot of one mutate-on-write write or
// recovery epoch. Completed write epochs retain confirmedValue so older
// in-flight syncs can be ignored.
type causalCaptureEpoch struct {
	confirmedAppliedEntry
}

func (e causalCaptureEpoch) IsPending() bool {
	return e.mode != modeConfirmed
}

func (e causalCaptureEpoch) IsAwaitingInitialSync() bool {
	return e.IsPending() && e.awaitingInitialSyncSeed
}

// BlocksSync reports whether this write epoch postdates syncStartedAt and must
// keep older in-flight sync snapshots from overwriting Running or capturing stale data.
func (e causalCaptureEpoch) BlocksSync(syncStartedAt int64) bool {
	return !e.awaitingInitialSyncSeed && syncStartedAt < e.captureAfterNano
}

type confirmedAppliedSnapshot struct {
	entries      map[canonicalLeafPath]confirmedAppliedEntry
	learnedPaths map[keylessLeafPath]struct{}
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
	if !ok || entry.confirmedValue == nil || !entry.matches(winningOwner, intendedValue) {
		return nil
	}
	return proto.Clone(entry.confirmedValue).(*sdcpb.TypedValue)
}

// Record stores confirmedValue as the baseline for (path, owner, intendedValue)
// when confirmedValue is non-empty. If intendedValue already matches the
// crypt/cipher allowlist and confirmedValue differs, any baseline for path is evicted.
func (s *confirmedAppliedValues) Record(path *sdcpb.Path, owner string, intendedValue, confirmedValue *sdcpb.TypedValue) {
	s.record(baselineIdentity{path: path, owner: owner, intendedValue: intendedValue}, confirmedValue, 0)
}

// RecordAfter stores a baseline and the write completion barrier that older
// in-flight sync snapshots must not cross, or evicts any baseline for path when
// a pre-hashed intendedValue differs from confirmedValue.
func (s *confirmedAppliedValues) RecordAfter(path *sdcpb.Path, owner string, intendedValue, confirmedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.record(baselineIdentity{path: path, owner: owner, intendedValue: intendedValue}, confirmedValue, captureAfterNano)
}

// RecordDiscovered seeds a baseline for an unannotated plain string leaf only
// when confirmedValue matches a recognized crypt/cipher format and intendedValue
// does not, learning the leaf's keyless schema path on match and evicting any
// prior baseline for path otherwise.
func (s *confirmedAppliedValues) RecordDiscovered(id baselineIdentity, confirmedValue *sdcpb.TypedValue) {
	if s == nil || id.path == nil || id.owner == "" || id.intendedValue == nil {
		return
	}
	key := newCanonicalLeafPath(id.path)
	s.mu.Lock()
	s.recordDiscoveredLocked(key, cloneBaselineIdentity(id), confirmedValue, 0)
	s.mu.Unlock()
}

func (s *confirmedAppliedValues) recordDiscoveredLocked(key canonicalLeafPath, clonedID baselineIdentity, observed *sdcpb.TypedValue, captureAfterNano int64) {
	if !isCryptDiscoveryMatch(clonedID.intendedValue, observed) {
		delete(s.entries, key)
		return
	}
	s.entries[key] = confirmedAppliedEntry{
		baselineIdentity: clonedID,
		confirmedValue:   proto.Clone(observed).(*sdcpb.TypedValue),
		captureAfterNano: captureAfterNano,
		mode:             modeConfirmed,
	}
	if kp := newKeylessLeafPath(clonedID.path); kp != "" {
		s.learnedPaths[kp] = struct{}{}
	}
}

func (s *confirmedAppliedValues) record(id baselineIdentity, confirmedValue *sdcpb.TypedValue, captureAfterNano int64) {
	if s == nil || id.path == nil || id.owner == "" || id.intendedValue == nil || !isNonEmptyTypedValue(confirmedValue) {
		return
	}
	if rejectsPreHashedMismatch(id.intendedValue, confirmedValue) {
		s.Delete(id.path)
		return
	}
	key := newCanonicalLeafPath(id.path)

	s.mu.Lock()
	s.entries[key] = confirmedAppliedEntry{
		baselineIdentity: cloneBaselineIdentity(id),
		confirmedValue:   proto.Clone(confirmedValue).(*sdcpb.TypedValue),
		captureAfterNano: captureAfterNano,
		mode:             modeConfirmed,
	}
	s.mu.Unlock()
}

// MarkPending retains the winning intent identity until a causally subsequent
// sync can supply the confirmed target value.
func (s *confirmedAppliedValues) MarkPending(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.markPending(baselineIdentity{path: path, owner: owner, intendedValue: intendedValue}, captureAfterNano, modeMutateOnWrite)
}

// MarkDiscoveryCandidate records an unconfirmed push marker for an unannotated
// plain string leaf whose first causal sync may reveal a crypt or cipher value.
func (s *confirmedAppliedValues) MarkDiscoveryCandidate(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue, captureAfterNano int64) {
	s.markPending(baselineIdentity{path: path, owner: owner, intendedValue: intendedValue}, captureAfterNano, modeCryptDiscovery)
}

// MarkAwaitingInitialSync defers baseline seeding for id until the first sync
// covering id.path completes. When preserveExisting is true, an existing
// baseline or pending marker at id.path is left intact.
func (s *confirmedAppliedValues) MarkAwaitingInitialSync(id baselineIdentity, mode baselineMode, preserveExisting bool) {
	if s == nil || id.path == nil || id.owner == "" || id.intendedValue == nil || mode == modeConfirmed {
		return
	}
	key := newCanonicalLeafPath(id.path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if preserveExisting {
		if _, exists := s.entries[key]; exists {
			return
		}
	}
	s.entries[key] = confirmedAppliedEntry{
		baselineIdentity:        cloneBaselineIdentity(id),
		awaitingInitialSyncSeed: true,
		mode:                    mode,
	}
}

func (s *confirmedAppliedValues) markPending(id baselineIdentity, captureAfterNano int64, mode baselineMode) {
	if s == nil || id.path == nil || id.owner == "" || id.intendedValue == nil || captureAfterNano <= 0 {
		return
	}
	key := newCanonicalLeafPath(id.path)
	s.mu.Lock()
	s.entries[key] = confirmedAppliedEntry{
		baselineIdentity: cloneBaselineIdentity(id),
		captureAfterNano: captureAfterNano,
		mode:             mode,
	}
	s.mu.Unlock()
}

// CausalCaptures returns write epochs that guard against older syncs plus
// pending post-Set or initial-sync seed epochs awaiting observation.
func (s *confirmedAppliedValues) CausalCaptures() []causalCaptureEpoch {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]causalCaptureEpoch, 0)
	for _, entry := range s.entries {
		if entry.captureAfterNano == 0 && !entry.awaitingInitialSyncSeed {
			continue
		}
		result = append(result, causalCaptureEpoch{confirmedAppliedEntry: cloneConfirmedAppliedEntry(entry)})
	}
	return result
}

// CompletePending resolves pending against the first covering observation:
// crypt-discovery epochs bind matching crypt/cipher observations and permanently
// learn the leaf's keyless schema path, mutate-on-write epochs bind non-empty
// observations unless a pre-hashed intendedValue differs, and non-matching or
// missing observations evict the epoch.
func (s *confirmedAppliedValues) CompletePending(pending causalCaptureEpoch, observed *sdcpb.TypedValue) {
	if s == nil || !pending.IsPending() {
		return
	}
	key := newCanonicalLeafPath(pending.path)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || entry.mode != pending.mode || entry.captureAfterNano != pending.captureAfterNano ||
		entry.awaitingInitialSyncSeed != pending.awaitingInitialSyncSeed ||
		!entry.matches(pending.owner, pending.intendedValue) {
		return
	}
	if pending.mode == modeCryptDiscovery {
		s.recordDiscoveredLocked(key, entry.baselineIdentity, observed, entry.captureAfterNano)
		return
	}
	if !isNonEmptyTypedValue(observed) || rejectsPreHashedMismatch(pending.intendedValue, observed) {
		delete(s.entries, key)
		return
	}
	entry.confirmedValue = proto.Clone(observed).(*sdcpb.TypedValue)
	entry.awaitingInitialSyncSeed = false
	entry.mode = modeConfirmed
	s.entries[key] = entry
}

// Snapshot captures a deep copy of all stored baselines and learned keyless
// paths so an unconfirmed transaction can restore untouched state on rollback.
func (s *confirmedAppliedValues) Snapshot() *confirmedAppliedSnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make(map[canonicalLeafPath]confirmedAppliedEntry, len(s.entries))
	for k, v := range s.entries {
		entries[k] = cloneConfirmedAppliedEntry(v)
	}
	return &confirmedAppliedSnapshot{
		entries:      entries,
		learnedPaths: maps.Clone(s.learnedPaths),
	}
}

// RestoreOnRollback reverts learnedPaths to snap (preserving any paths confirmed
// by live write epochs) and restores untouched pre-transaction entries whose
// owner and intendedValue are once again active.
func (s *confirmedAppliedValues) RestoreOnRollback(
	snap *confirmedAppliedSnapshot,
	isTouched func(path *sdcpb.Path) bool,
	isActiveWinner func(id baselineIdentity) bool,
) {
	if s == nil || snap == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	restoredLearned := maps.Clone(snap.learnedPaths)
	for _, entry := range s.entries {
		if entry.mode == modeConfirmed && entry.captureAfterNano > 0 {
			if kp := newKeylessLeafPath(entry.path); kp != "" {
				restoredLearned[kp] = struct{}{}
			}
		}
	}
	s.learnedPaths = restoredLearned

	for key, prev := range snap.entries {
		if _, exists := s.entries[key]; exists {
			continue
		}
		if isTouched(prev.path) || !isActiveWinner(prev.baselineIdentity) {
			continue
		}
		s.entries[key] = cloneConfirmedAppliedEntry(prev)
	}
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

// SnapshotIdentities returns cloned identities for all currently stored baselines
// and pending capture markers.
func (s *confirmedAppliedValues) SnapshotIdentities() []baselineIdentity {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return nil
	}
	out := make([]baselineIdentity, 0, len(s.entries))
	for _, entry := range s.entries {
		out = append(out, cloneBaselineIdentity(entry.baselineIdentity))
	}
	return out
}

// DeleteIdentity removes the entry at id.path only when its stored owner and
// intendedValue still match id.
func (s *confirmedAppliedValues) DeleteIdentity(id baselineIdentity) {
	if s == nil || id.path == nil {
		return
	}
	key := newCanonicalLeafPath(id.path)
	s.mu.Lock()
	if entry, ok := s.entries[key]; ok && entry.matches(id.owner, id.intendedValue) {
		delete(s.entries, key)
	}
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
