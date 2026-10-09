package sensitive

import (
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// PathChecker is the read-only view of the sensitive path markers consumed by
// Render. *PathIndex satisfies it; nil means "no markers".
type PathChecker interface {
	Contains(path *sdcpb.Path) bool
}

var _ PathChecker = (*PathIndex)(nil)

// PathIndex is the datastore-wide, concurrent-safe live index (the Live Sensitive Path Index) of
// sensitive schema paths. Use Set/Delete for incremental per-intent updates.
// The map value tracks which intents own each path so Delete can surgically
// remove only that intent's contribution.
type PathIndex struct {
	mu    sync.RWMutex
	index map[string][]string // key-pruned XPath → intent names
}

// NewPathIndex returns a ready-to-use, empty PathIndex.
func NewPathIndex() *PathIndex {
	return &PathIndex{index: make(map[string][]string)}
}

// Set replaces the sensitive paths contributed by intentName with paths.
// Existing entries for intentName are removed before the new ones are added.
// Safe to call concurrently.
func (s *PathIndex) Set(intentName string, paths []*sdcpb.Path) {
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
func (s *PathIndex) Delete(intentName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(intentName)
}

// removeLocked removes intentName from every entry in the index.
// Entries whose slice becomes empty are deleted from the map.
// Caller must hold the write lock.
func (s *PathIndex) removeLocked(intentName string) {
	for key, names := range s.index {
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
func (s *PathIndex) Contains(path *sdcpb.Path) bool {
	if s == nil || path == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.index[path.ToXPath(true)]
	return ok
}
