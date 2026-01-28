package tree

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// LeafEntry stores the *cache.Update along with additional attributes.
// These Attributes indicate if the entry is to be deleted / added (new) or updated.
type LeafEntry struct {
	*types.Update

	// helper values
	parentEntry        Entry
	IsNew              bool
	Delete             bool
	DeleteOnlyIntended bool
	IsUpdated          bool
	IsExplicitDelete   bool

	mu sync.RWMutex
}

func (l *LeafEntry) DeepCopy(parentEntry Entry) *LeafEntry {
	upd := l.Update.DeepCopy()
	upd.SetParent(parentEntry)
	return &LeafEntry{
		Update:             upd,
		parentEntry:        parentEntry,
		IsNew:              l.IsNew,
		Delete:             l.Delete,
		DeleteOnlyIntended: l.DeleteOnlyIntended,
		IsUpdated:          l.IsUpdated,
		IsExplicitDelete:   l.IsExplicitDelete,
		mu:                 sync.RWMutex{},
	}
}

func (l *LeafEntry) GetEntry() Entry {
	return l.parentEntry
}

// MarkUpdate indicate that the entry is an Updated value
func (l *LeafEntry) MarkUpdate(u *types.Update) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// set the new value
	l.Update = u
	// set the update flag
	l.IsUpdated = true
	// reset the delete flag
	l.Delete = false
	l.IsNew = false
}

func (l *LeafEntry) MarkNew() {
	l.mu.Lock()
	defer l.mu.Unlock()
	// set the update flag
	l.IsUpdated = false
	// reset the delete flag
	l.Delete = false
	l.DeleteOnlyIntended = false
	l.IsNew = true
}

func (l *LeafEntry) RemoveDeleteFlag() *LeafEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = false
	return l
}

func (l *LeafEntry) GetDeleteFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Delete
}

func (l *LeafEntry) GetExplicitDeleteFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.IsExplicitDelete
}

func (l *LeafEntry) GetDeleteOnlyIntendedFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.DeleteOnlyIntended
}

func (l *LeafEntry) GetUpdateFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.IsUpdated
}

func (l *LeafEntry) GetNewFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.IsNew
}

func (l *LeafEntry) GetUpdate() *types.Update {
	return l.Update
}

func (l *LeafEntry) DropDeleteFlag() *LeafEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = false
	l.DeleteOnlyIntended = false
	return l
}

// MarkDelete indicate that the entry is to be deleted
func (l *LeafEntry) MarkDelete(onlyIntended bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = true
	if onlyIntended {
		l.DeleteOnlyIntended = true
	}
	l.IsUpdated = false
	l.IsNew = false
}

// MarkExpliciteDelete indicate that the entry is to be explicitely deleted
func (l *LeafEntry) MarkExpliciteDelete() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = true
	l.IsExplicitDelete = true
	l.IsUpdated = false
	l.IsNew = false
}

func (l *LeafEntry) NonRevertive() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	// this is a hack that makes the tests pass
	if l.parentEntry == nil {
		return false
	}
	return l.parentEntry.getTreeContext().IsNonRevertiveIntent(l.Owner())
}

// String returns a string representation of the LeafEntry
func (l *LeafEntry) String() string {
	return fmt.Sprintf("Owner: %s, Priority: %d, Value: %s, New: %t, Delete: %t, Update: %t, DeleteIntendedOnly: %t, ExplicitDelete: %t, Non-Revertive: %t", l.Owner(), l.Priority(), l.Value().ToString(), l.GetNewFlag(), l.GetDeleteFlag(), l.GetUpdateFlag(), l.GetDeleteOnlyIntendedFlag(), l.GetExplicitDeleteFlag(), l.NonRevertive())
}

// Compare used for slices.SortFunc. Sorts by path and if equal paths then by owner as the second criteria
func (l *LeafEntry) Compare(other *LeafEntry) int {
	result := sdcpb.ComparePath(l.Path(), other.Path())
	if result != 0 {
		return result
	}
	return strings.Compare(l.Update.Owner(), other.Update.Owner())
}

// NewLeafEntry constructor for a new LeafEntry
func NewLeafEntry(c *types.Update, flags *types.UpdateInsertFlags, parent Entry) *LeafEntry {
	le := &LeafEntry{
		parentEntry: parent,
		Update:      c,
	}
	c.SetParent(parent)
	flags.Apply(le)
	return le
}

func (l *LeafEntry) Equal(other *LeafEntry) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	equal := l.Update.Equal(other.Update)
	if !equal {
		return false
	}
	return l.Delete == other.Delete && l.IsUpdated == other.IsUpdated && l.IsNew == other.IsNew && l.IsExplicitDelete == other.IsExplicitDelete && l.DeleteOnlyIntended == other.DeleteOnlyIntended
}
