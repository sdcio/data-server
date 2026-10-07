package datastore

import (
	"sync"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// driftRevertState tracks an unfinished drift revert so that a later sync can retry it,
// even when that sync observes no new Running changes.
// The outstanding marker and the scopes are guarded by a single mutex so they are always
// observed consistently.
type driftRevertState struct {
	mu          sync.Mutex
	outstanding bool
	scopes      *sdcpb.PathSet
}

// Outstanding reports whether a previous revert is still unfinished.
func (s *driftRevertState) Outstanding() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outstanding
}

// Pending returns a copy of the scopes last evaluated for revert.
func (s *driftRevertState) Pending() *sdcpb.PathSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scopes == nil {
		return sdcpb.NewPathSet()
	}
	return s.scopes.DeepCopy()
}

// Begin records the scopes being reverted, so a failed attempt can be retried.
func (s *driftRevertState) Begin(scopes *sdcpb.PathSet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scopes == nil {
		s.scopes = nil
		return
	}
	s.scopes = scopes.DeepCopy()
}

// Fail marks the revert as unfinished; the recorded scopes are kept for the retry.
func (s *driftRevertState) Fail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outstanding = true
}

// Done marks the revert as complete and clears the recorded scopes.
func (s *driftRevertState) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outstanding = false
	s.scopes = nil
}
