package tree

import (
	"iter"
	"math"
	"sync"

	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	log "github.com/sirupsen/logrus"
)

type LeafVariants struct {
	les         LeafVariantSlice
	lesMutex    sync.RWMutex
	tc          *TreeContext
	parentEntry Entry
}

func newLeafVariants(tc *TreeContext, parentEnty Entry) *LeafVariants {
	return &LeafVariants{
		les:         make(LeafVariantSlice, 0, 2),
		tc:          tc,
		parentEntry: parentEnty,
	}
}

func (lv *LeafVariants) Add(le *LeafEntry) {
	if leafVariant := lv.GetByOwner(le.Owner()); leafVariant != nil {
		if leafVariant.Equal(le.Update) {
			// it seems like the element was not deleted, so drop the delete flag
			leafVariant.DropDeleteFlag()
		} else {
			// if a leafentry of the same owner exists with different value, mark it for update
			leafVariant.MarkUpdate(le.Update)
		}
	} else {
		lv.lesMutex.Lock()
		defer lv.lesMutex.Unlock()
		// if LeafVaraint with same owner does not exist, add the new entry
		lv.les = append(lv.les, le)
	}
}

// Items iterator for the LeafVariants
func (lv *LeafVariants) Items() iter.Seq[*LeafEntry] {
	return func(yield func(*LeafEntry) bool) {
		lv.lesMutex.RLock()
		defer lv.lesMutex.RUnlock()
		for _, v := range lv.les {
			if !yield(v) {
				return
			}
		}
	}
}

func (lv *LeafVariants) Length() int {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	return len(lv.les)
}

func (lv *LeafVariants) canDeleteBranch(keepDefault bool) bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()

	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return true
	}

	// go through all variants
	for _, l := range lv.les {
		// if the LeafVariant is not owned by running or default
		if l.Update.Owner() != DefaultsIntentName || keepDefault {
			// then we need to check that it remains, so not Delete Flag set or DeleteOnylIntended Flags set [which results in not doing a delete towards the device]
			if l.GetDeleteOnlyIntendedFlag() || !l.GetDeleteFlag() {
				// then this entry should not be deleted
				return false
			}
		}
	}
	return true
}

// canDelete returns true if leafValues exist that are not owned by default or running that do not have the DeleteFlag set [or if delete is set, also the DeleteOnlyIntendedFlag set]
func (lv *LeafVariants) canDelete() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return true
	}

	// if we have runnig and only running we should not delete
	if len(lv.les) == 1 && lv.les[0].Owner() == RunningIntentName {
		return false
	}

	// go through all variants
	for _, l := range lv.les {
		// if the LeafVariant is not owned by running or default
		if l.Update.Owner() != RunningIntentName && l.Update.Owner() != DefaultsIntentName {
			// then we need to check that it remains, so not Delete Flag set or DeleteOnylIntended Flags set [which results in not doing a delete towards the device]
			if l.GetDeleteOnlyIntendedFlag() || !l.GetDeleteFlag() {
				// then this entry should not be deleted
				return false
			}
		}
	}
	return true
}

// shouldDelete evaluates the LeafVariants and indicates if the overall result is, that the Entry referencing these
// LeafVariants is explicitly to be deleted. Meaning there are no other LeafVariants remaining after the pending action, that
// any LeafVariant other then Running or Defaults exist.
func (lv *LeafVariants) shouldDelete() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return false
	}

	foundOtherThenRunningAndDefault := false
	// go through all variants
	for _, l := range lv.les {
		// if an entry exists that is not owned by running or default,
		if l.Update.Owner() == RunningIntentName || l.Update.Owner() == DefaultsIntentName {
			continue
		}
		foundOtherThenRunningAndDefault = true

		// if an entry exists that has
		// the only intended flag set or not the Delete Flag and is not owned by default and not owned by running
		if l.GetDeleteOnlyIntendedFlag() || !l.GetDeleteFlag() {
			// then this entry should not be deleted
			return false
		}
	}
	if !foundOtherThenRunningAndDefault {
		return false
	}
	return true
}

func (lv *LeafVariants) remainsToExist() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return false
	}

	// go through all variants
	for _, l := range lv.les {
		// if an entry exists that does not have the delete flag set,
		// then a remaining LeafVariant exists.
		if !l.GetDeleteFlag() {
			return true
		}
	}
	return false
}

func (lv *LeafVariants) GetHighestPrecedenceValue(filter HighestPrecedenceFilter) int32 {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	result := int32(math.MaxInt32)
	for _, e := range lv.les {
		if filter(e) && e.Owner() != DefaultsIntentName && e.Update.Priority() < result {
			result = e.Update.Priority()
		}
	}
	return result
}

func (lv *LeafVariants) DeepCopy(tc *TreeContext, parent Entry) *LeafVariants {
	result := &LeafVariants{
		lesMutex:    sync.RWMutex{},
		tc:          tc,
		les:         make([]*LeafEntry, 0, len(lv.les)),
		parentEntry: parent,
	}

	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	for _, x := range lv.les {
		result.Add(x.DeepCopy(parent))
	}

	return result
}

// checkReturnDefault checks if defaults are allowed and if the given LeafEntry is owned by default
func checkNotDefaultAllowedButIsDefaultOwner(le *LeafEntry, includeDefaults bool) bool {
	return !includeDefaults && le.Update.Owner() == DefaultsIntentName
}

func checkExistsAndDeleteFlagSet(le *LeafEntry) bool {
	return le != nil && le.GetDeleteFlag()
}

func checkNewOrUpdateFlagSet(le *LeafEntry) bool {
	return le.GetNewFlag() || le.GetUpdateFlag()
}

func checkNotOwner(le *LeafEntry, owner string) bool {
	return le.Owner() != owner
}

func (lv *LeafVariants) GetRunning() *LeafEntry {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	for _, e := range lv.les {
		if e.Update.Owner() == RunningIntentName {
			return e
		}
	}
	return nil
}

// GetHighesNewUpdated returns the LeafEntry with the highes priority
// nil if no leaf entry exists.
func (lv *LeafVariants) GetHighestPrecedence(onlyNewOrUpdated bool, includeDefaults bool) *LeafEntry {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	if len(lv.les) == 0 {
		return nil
	}
	if onlyNewOrUpdated && lv.shouldDelete() {
		return nil
	}

	var highest *LeafEntry
	var secondHighest *LeafEntry
	for _, e := range lv.les {
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

	// do not include defaults loaded at validation time
	if checkNotDefaultAllowedButIsDefaultOwner(highest, includeDefaults) {
		return nil
	}

	// if it does not matter if the highes update is also
	// New or Updated return it
	if !onlyNewOrUpdated {
		return highest
	}

	// if the highes is not marked for deletion and new or updated (=PrioChanged) return it
	if !checkExistsAndDeleteFlagSet(highest) {
		if checkNewOrUpdateFlagSet(highest) || lv.highestIsUnequalRunning(highest) {
			return highest
		}
		return nil
	}
	// otherwise if the secondhighest is not marked for deletion return it
	if !checkExistsAndDeleteFlagSet(secondHighest) && checkNotOwner(secondHighest, RunningIntentName) {
		return secondHighest
	}

	// otherwise return nil
	return nil
}

func (lv *LeafVariants) highestIsUnequalRunning(highest *LeafEntry) bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// if highes is already running or even default, return false
	if highest.Update.Owner() == RunningIntentName {
		return false
	}

	runVal := lv.GetByOwner(RunningIntentName)
	if runVal == nil {
		return true
	}

	// ignore errors, they should not happen :-P I know... should...
	rval := runVal.Value()
	hval := highest.Value()

	return !utils.EqualTypedValues(rval, hval)
}

// GetByOwner returns the entry that is owned by the given owner,
// returns nil if no entry exists.
func (lv *LeafVariants) GetByOwner(owner string) *LeafEntry {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	for _, e := range lv.les {
		if e.Owner() == owner {
			return e
		}
	}
	return nil
}

// MarkOwnerForDeletion searches for a LefVariant of given owner, if it exists
// the entry is marked for deletion.
// returning the leafentry if an owner entry was found, nil if not.
func (lv *LeafVariants) MarkOwnerForDeletion(owner string, onlyIntended bool) *LeafEntry {
	le := lv.GetByOwner(owner)
	if le != nil {
		le.MarkDelete(onlyIntended)
		return le
	}
	return nil
}

func (lv *LeafVariants) DeleteByOwner(owner string) {
	lv.lesMutex.Lock()
	defer lv.lesMutex.Unlock()
	for i, l := range lv.les {
		if l.Owner() == owner {
			// Remove element from slice
			lv.les = append(lv.les[:i], lv.les[i+1:]...)
			break
		}
	}
}

func (lv *LeafVariants) GetDeviations(ch chan<- *types.DeviationEntry, isActiveCase bool) {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()

	if len(lv.les) == 0 {
		return
	}

	// get the path via the first LeafEntry
	// is valid for all entries
	sdcpbPath, err := lv.parentEntry.SdcpbPath()
	if err != nil {
		log.Error(err)
	}

	// we are part of an inactive case of a choice
	if !isActiveCase {
		for _, le := range lv.les {
			ch <- types.NewDeviationEntry(le.Owner(), types.DeviationReasonOverruled, sdcpbPath).SetExpectedValue(le.Value())
		}
		return
	}

	var running *LeafEntry
	var highest *LeafEntry

	overruled := make([]*types.DeviationEntry, 0, len(lv.les))
	for _, le := range lv.les {
		// Defaults should be skipped
		if le.Owner() == DefaultsIntentName {
			continue
		}
		// running is stored in running var
		if le.Owner() == RunningIntentName {
			running = le
			continue
		}
		// if no highest exists yet, set it
		if highest == nil {
			highest = le
			continue
		}
		// if precedence of actual (le) is higher then highest
		// replace highest with it
		if le.Priority() < highest.Priority() {
			de := types.NewDeviationEntry(highest.Owner(), types.DeviationReasonOverruled, sdcpbPath).SetExpectedValue(highest.Value())
			overruled = append(overruled, de)
			highest = le
		}
		// if precedence of actual (le) is lower then le needs to be added to overruled
		if le.Priority() >= highest.Priority() {
			de := types.NewDeviationEntry(le.Owner(), types.DeviationReasonOverruled, sdcpbPath).SetExpectedValue(le.Value())
			overruled = append(overruled, de)
		}
	}

	// send all the overruleds
	for _, de := range overruled {
		ch <- de.SetCurrentValue(highest.Value())
	}

	// if there is no running and no highest (probably a default), skip
	if running == nil && highest == nil {
		return
	}

	// unhandled -> running but no intent data
	if running != nil && highest == nil {
		ch <- types.NewDeviationEntry(running.Owner(), types.DeviationReasonUnhandled, sdcpbPath).SetCurrentValue(running.Value())
		return
	}

	// if highest exists but not running  OR   running != highest
	if (running == nil && highest != nil) || running.Value().Cmp(highest.Value()) != 0 {
		de := types.NewDeviationEntry(highest.Owner(), types.DeviationReasonNotApplied, sdcpbPath).SetExpectedValue(highest.Value())
		if running != nil {
			de.SetCurrentValue(running.Value())
		}
		ch <- de
	}

}
