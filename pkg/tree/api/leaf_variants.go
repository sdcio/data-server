package api

import (
	"context"
	"iter"
	"math"
	"sync"

	. "github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type LeafVariants struct {
	les         LeafVariantSlice
	lesMutex    sync.RWMutex
	tc          TreeContext
	parentEntry Entry
}

func NewLeafVariants(tc TreeContext, parentEnty Entry) *LeafVariants {
	return &LeafVariants{
		les:         make(LeafVariantSlice, 0, 2),
		tc:          tc,
		parentEntry: parentEnty,
	}
}

func (lv *LeafVariants) AddWithStats(le *LeafEntry, stats *types.ImportStats) {
	if leafVariant := lv.GetByOwner(le.Owner()); leafVariant != nil {
		if leafVariant.Update.Equal(le.Update) {
			// it seems like the element was not deleted, so drop the delete flag
			leafVariant.DropDeleteFlag()
		} else {
			// if a leafentry of the same owner exists with different value, mark it for update
			leafVariant.MarkUpdate(le.Update)
			stats.IncrementUpdated()

		}
	} else {
		lv.lesMutex.Lock()
		defer lv.lesMutex.Unlock()
		// if LeafVaraint with same owner does not exist, add the new entry
		lv.les = append(lv.les, le)
		stats.IncrementNew()
	}
}

func (lv *LeafVariants) Add(le *LeafEntry) {
	lv.AddWithStats(le, nil)
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

func (lv *LeafVariants) CanDeleteBranch(keepDefault bool) bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()

	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return true
	}

	highest := lv.GetHighestPrecedence(false, false, true)
	if highest != nil && highest.IsExplicitDelete {
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

// RemoveDeletedByOwner removes and returns the LeafEntry owned by the given owner if it is marked for deletion.
func (lv *LeafVariants) RemoveDeletedByOwner(owner string) *LeafEntry {
	lv.lesMutex.Lock()
	defer lv.lesMutex.Unlock()
	for i, l := range lv.les {
		if l.Owner() == owner && l.GetDeleteFlag() {
			// Remove element from slice
			lv.les = append(lv.les[:i], lv.les[i+1:]...)
			return l
		}
	}
	return nil
}

// checkOnlyRunningAndMaybeDefault checks if only running and maybe default LeafVariants exist
func (lv *LeafVariants) checkOnlyRunningAndMaybeDefault() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()

	if len(lv.les) == 1 && lv.les[0].Owner() == RunningIntentName {
		return true
	}

	// check if only running and default exist
	hasRunning := false
	hasDefault := false
	if len(lv.les) == 2 {
		for _, l := range lv.les {
			switch l.Owner() {
			case RunningIntentName:
				hasRunning = true
			case DefaultsIntentName:
				hasDefault = true
			}
		}
	}
	return hasRunning && hasDefault
}

// CanDelete returns true if leafValues exist that are not owned by default or running that do not have the DeleteFlag set [or if delete is set, also the DeleteOnlyIntendedFlag set]
func (lv *LeafVariants) CanDelete() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return true
	}

	// if we have runnig and only running (or default in addition) we should not delete
	if lv.checkOnlyRunningAndMaybeDefault() {
		return false
	}

	// check if highest is explicit delete
	highest := lv.GetHighestPrecedence(false, false, true)
	if highest != nil && highest.IsExplicitDelete {
		return true
	}

	// go through all variants
	for _, l := range lv.les {
		// if the LeafVariant is not owned by running or default
		if l.Update.Owner() != RunningIntentName && l.Update.Owner() != DefaultsIntentName && !l.IsExplicitDelete {
			// then we need to check that it remains, so not Delete Flag set or DeleteOnylIntended Flags set [which results in not doing a delete towards the device]
			if l.GetDeleteOnlyIntendedFlag() || !l.GetDeleteFlag() {
				// then this entry should not be deleted
				return false
			}
		}
	}
	return true
}

// ShouldDelete evaluates the LeafVariants and indicates if the overall result is, that the Entry referencing these
// LeafVariants is explicitly to be deleted. Meaning there are no other LeafVariants remaining after the pending action, that
// any LeafVariant other then Running or Defaults exist.
func (lv *LeafVariants) ShouldDelete() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return false
	}

	// check if highest is explicit delete
	highest := lv.GetHighestPrecedence(false, false, true)
	if highest != nil && highest.IsExplicitDelete && lv.GetRunning() != nil {
		return true
	}

	// if there is no running, no need to delete
	if lv.GetRunning() == nil {
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
	return foundOtherThenRunningAndDefault
}

func (lv *LeafVariants) RemainsToExist() bool {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	// only procede if we have leave variants
	if len(lv.les) == 0 {
		return false
	}

	highest := lv.GetHighestPrecedence(false, false, true)

	if highest == nil || highest.IsExplicitDelete {
		return false
	}

	defaultOrRunningExists := false
	deleteExists := false
	// go through all variants
	for _, l := range lv.les {
		if l.Owner() == RunningIntentName || l.Owner() == DefaultsIntentName {
			defaultOrRunningExists = true
			continue
		}
		// if an entry exists that does not have the delete flag set,
		// then a remaining LeafVariant exists.
		if !l.GetDeleteFlag() {
			return true
		}
		deleteExists = true
	}

	if deleteExists {
		return false
	}
	return defaultOrRunningExists
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

func (lv *LeafVariants) DeepCopy(tc TreeContext, parent Entry) *LeafVariants {
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
func (lv *LeafVariants) GetHighestPrecedence(onlyNewOrUpdated bool, includeDefaults bool, includeExplicitDelete bool) *LeafEntry {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()
	if len(lv.les) == 0 {
		return nil
	}
	if onlyNewOrUpdated && lv.CanDelete() {
		return nil
	}

	// figure out the highest precedence LeafEntry
	var highest *LeafEntry
	// the second highests is the backup in case the highest is marked for deletion
	// so this is not actually the second highest always, but the next candidate
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
			if secondHighest == nil || secondHighest.Priority() > e.Priority() && !e.GetDeleteFlag() {
				secondHighest = e
			}
		}
	}

	if highest == nil || highest.IsExplicitDelete && !includeExplicitDelete {
		return nil
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
	if secondHighest != nil && !checkExistsAndDeleteFlagSet(secondHighest) && checkNotOwner(secondHighest, RunningIntentName) {
		return secondHighest
	}

	// otherwise return nil
	return nil
}

// highestIsUnequalRunning checks if the highest precedence LeafEntry is unequal to the running LeafEntry
// Expects the caller to hold the read lock on lesMutex.
func (lv *LeafVariants) highestIsUnequalRunning(highest *LeafEntry) bool {
	// if highes is already running or even default, return false
	if highest.Update.Owner() == RunningIntentName {
		return false
	}

	// if highest is not new or updated and highest is non-revertive
	if !highest.IsNew && !highest.IsUpdated && lv.tc.NonRevertiveInfo().IsGenerallyNonRevertive(highest.Update.Owner()) {
		return false
	}

	runVal := lv.GetByOwner(RunningIntentName)
	if runVal == nil {
		return true
	}

	// ignore errors, they should not happen :-P I know... should...
	rval := runVal.Value()
	hval := highest.Value()
	return !rval.Equal(hval)
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

func (lv *LeafVariants) ResetFlags(deleteFlag bool, newFlag bool, updatedFlag bool) int {
	lv.lesMutex.Lock()
	defer lv.lesMutex.Unlock()
	count := 0

	for _, le := range lv.les {
		if deleteFlag && le.Delete {
			le.Delete = false
			le.DeleteOnlyIntended = false
			count++
		}
		if updatedFlag && le.IsUpdated {
			le.IsUpdated = false
			count++
		}
		if newFlag && le.IsNew {
			le.IsNew = false
			count++
		}
	}
	return count
}

func (lv *LeafVariants) DeleteByOwner(owner string) *LeafEntry {
	lv.lesMutex.Lock()
	defer lv.lesMutex.Unlock()
	for i, l := range lv.les {
		if l.Owner() == owner {
			// Remove element from slice
			lv.les = append(lv.les[:i], lv.les[i+1:]...)
			return l
		}
	}
	return nil
}

func (lv *LeafVariants) GetDeviations(ctx context.Context, ch chan<- *types.DeviationEntry, isActiveCase bool) {
	lv.lesMutex.RLock()
	defer lv.lesMutex.RUnlock()

	if len(lv.les) == 0 {
		return
	}

	// get the path via the first LeafEntry
	// is valid for all entries
	sdcpbPath := lv.parentEntry.SdcpbPath()

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
		if de.ExpectedValue().Equal(highest.Value()) {
			// skip if higher prio equals the overruled
			continue
		}
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
	if (running == nil && highest != nil) || !running.Value().Equal(highest.Value()) {
		de := types.NewDeviationEntry(highest.Owner(), types.DeviationReasonNotApplied, sdcpbPath).SetExpectedValue(highest.Value())
		if running != nil {
			de.SetCurrentValue(running.Value())
		}
		ch <- de
	}

}

func (lv *LeafVariants) AddExplicitDeleteEntry(intentName string, priority int32) *LeafEntry {
	le := NewLeafEntry(types.NewUpdate(lv.parentEntry, &sdcpb.TypedValue{}, priority, intentName, 0), types.NewUpdateInsertFlags().SetExplicitDeleteFlag(), lv.parentEntry)
	lv.Add(le)
	return le
}
