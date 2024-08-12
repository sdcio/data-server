package tree

import (
	"fmt"

	"github.com/sdcio/data-server/pkg/cache"
)

// LeafEntry stores the *cache.Update along with additional attributes.
// These Attributes indicate if the entry is to be deleted / added (new) or updated.
type LeafEntry struct {
	*cache.Update
	IsNew     bool
	Delete    bool
	IsUpdated bool
}

// MarkUpdate indicate that the entry is an Updated value
func (l *LeafEntry) MarkUpdate(u *cache.Update) {
	// set the new value
	l.Update = u
	// set the update flag
	l.IsUpdated = true
	// reset the delete flag
	l.Delete = false
}

// MarkDelete indicate that the entry is to be deleted
func (l *LeafEntry) MarkDelete() {
	l.Delete = true
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
func NewLeafEntry(c *cache.Update, new bool) *LeafEntry {
	return &LeafEntry{
		Update: c,
		IsNew:  new,
	}
}
