package types

import (
	"fmt"
	"sync/atomic"
)

type ImportStats struct {
	newEntries     atomic.Int64
	updatedEntries atomic.Int64
}

func NewImportStats() *ImportStats {
	return &ImportStats{}
}

func (is *ImportStats) String() string {
	return fmt.Sprintf("NewEntries: %d, UpdatedEntries: %d", is.newEntries.Load(), is.updatedEntries.Load())
}

func (is *ImportStats) Join(i *ImportStats) {
	is.newEntries.Add(i.newEntries.Load())
	is.updatedEntries.Add(i.updatedEntries.Load())
}

func (is *ImportStats) IncrementNew() {
	if is == nil {
		return
	}
	is.newEntries.Add(1)
}

func (is *ImportStats) IncrementUpdated() {
	if is == nil {
		return
	}
	is.updatedEntries.Add(1)
}

func (is *ImportStats) GetNewCount() int64 {
	return is.newEntries.Load()
}

func (is *ImportStats) GetUpdatedCount() int64 {
	return is.updatedEntries.Load()
}

func (is *ImportStats) Changed() bool {
	return is.newEntries.Load() > 0 || is.updatedEntries.Load() > 0
}
