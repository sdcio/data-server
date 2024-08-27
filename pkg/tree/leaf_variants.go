package tree

import "math"

type LeafVariants []*LeafEntry

func newLeafVariants() LeafVariants {
	return make([]*LeafEntry, 0)
}

// ShouldDelete indicates if the entry should be deleted,
// since it is an entry that represents LeafsVariants but non
// of these are still valid.
func (lv LeafVariants) shouldDelete() bool {
	// only procede if we have leave variants
	if len(lv) == 0 {
		return false
	}

	// go through all variants
	for _, e := range lv {
		// if there is a variant that is not marked as delete, no delete should be issued
		if !e.Delete {
			return false
		}
	}
	// return true otherwise
	return true
}

func (lv LeafVariants) GetHighestPrecedenceValue() int32 {
	result := int32(math.MaxInt32)
	for _, e := range lv {
		if !e.Delete && e.Update.Priority() < result {
			result = e.Update.Priority()
		}
	}
	return result
}

// GetHighesNewUpdated returns the LeafEntry with the highes priority
// nil if no leaf entry exists.
func (lv LeafVariants) GetHighestPrecedence(onlyIfPrioChanged bool) *LeafEntry {
	if len(lv) == 0 {
		return nil
	}

	var highest *LeafEntry
	var secondHighest *LeafEntry
	for _, e := range lv {
		// first entry set result to it
		// if it is not marked for deletion
		if highest == nil {
			highest = e
			continue
		}
		// on a result != nil that is then not marked for deletion
		// start comparing priorities and choose the one with the
		// higher prio (lower number)
		if highest.Priority() > e.Priority() {
			secondHighest = highest
			highest = e
		} else {
			// check if the update is at least higher prio (lower number) then the secondHighest
			if secondHighest == nil || secondHighest.Priority() > e.Priority() {
				secondHighest = e
			}
		}
	}

	// if it does not matter if the highes update is also
	// New or Updated return it
	if !onlyIfPrioChanged {
		if !highest.Delete {
			return highest
		}
		return secondHighest
	}

	// if the highes is not marked for deletion and new or updated (=PrioChanged) return it
	if !highest.Delete {
		if highest.IsNew || highest.IsUpdated {
			return highest
		}
		return nil
	}
	// otherwise if the secondhighest is not marked for deletion return it
	if secondHighest != nil && !secondHighest.Delete {
		return secondHighest
	}

	// otherwise return nil
	return nil
}

// GetByOwner returns the entry that is owned by the given owner,
// returns nil if no entry exists.
func (lv LeafVariants) GetByOwner(owner string) *LeafEntry {
	for _, e := range lv {
		if e.Owner() == owner {
			return e
		}
	}
	return nil
}
