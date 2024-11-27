package tree

import (
	"fmt"
	"sync"

	"github.com/sdcio/data-server/pkg/cache"
)

// LeafEntry stores the *cache.Update along with additional attributes.
// These Attributes indicate if the entry is to be deleted / added (new) or updated.
type LeafEntry struct {
	*cache.Update
	parentEntry Entry
	IsNew       bool
	Delete      bool
	IsUpdated   bool
	mu          sync.RWMutex
}

func (l *LeafEntry) GetEntry() Entry {
	return l.parentEntry
}

// MarkUpdate indicate that the entry is an Updated value
func (l *LeafEntry) MarkUpdate(u *cache.Update) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// set the new value
	l.Update = u
	// set the update flag
	l.IsUpdated = true
	// reset the delete flag
	l.Delete = false
}

func (l *LeafEntry) GetDeleteFlag() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Delete
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

func (l *LeafEntry) DropDeleteFlag() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = false
}

// MarkDelete indicate that the entry is to be deleted
func (l *LeafEntry) MarkDelete() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Delete = true
	l.IsUpdated = false
}

func (l *LeafEntry) GetRootBasedEntryChain() []Entry {
	return l.parentEntry.GetRootBasedEntryChain()
}

// String returns a string representation of the LeafEntry
func (l *LeafEntry) String() string {
	tv, err := l.Value()
	var v string
	if err != nil {
		v = err.Error()
	} else {
		v = tv.String()
	}
	return fmt.Sprintf("Owner: %s, Priority: %d, Value: %s, New: %t, Delete: %t, Update: %t", l.Owner(), l.Priority(), v, l.IsNew, l.Delete, l.IsUpdated)
}

// NewLeafEntry constructor for a new LeafEntry
func NewLeafEntry(c *cache.Update, new bool, parent Entry) *LeafEntry {
	return &LeafEntry{
		parentEntry: parent,
		Update:      c,
		IsNew:       new,
	}
}
