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
	"context"

	"github.com/sdcio/data-server/pkg/datastore/types"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// learnedPathOverlay combines permanently learned mutate-on-write paths with
// validation-scoped paths discovered on a single transaction's tree.
type learnedPathOverlay struct {
	base      api.LearnedMutateOnWritePathSet
	transient map[keylessLeafPath]struct{}
}

func (o learnedPathOverlay) IsLearnedMutateOnWritePath(path *sdcpb.Path) bool {
	if o.base != nil && o.base.IsLearnedMutateOnWritePath(path) {
		return true
	}
	_, ok := o.transient[newKeylessLeafPath(path)]
	return ok
}

// ensureColdStartCandidatesLoaded registers pre-existing Non-revertive intents
// from cacheClient as awaiting initial-sync baseline seeding once per Datastore
// lifetime, before the first sync or write transaction modifies cache state.
// Callers must not hold syncTreeMutex.
func (d *Datastore) ensureColdStartCandidatesLoaded(ctx context.Context) error {
	if d == nil || d.confirmedApplied == nil || d.cacheClient == nil || d.schemaClient == nil {
		return nil
	}
	d.coldStartMu.Lock()
	defer d.coldStartMu.Unlock()
	if d.coldStartLoaded {
		return nil
	}

	intentTree, err := d.NewEmptyTree(ctx)
	if err != nil {
		return err
	}
	intentNames, err := d.LoadAllButRunningIntents(ctx, intentTree)
	if err != nil {
		return err
	}
	if len(intentNames) == 0 {
		d.coldStartLoaded = true
		return nil
	}
	hasNonRevertive := false
	for _, name := range intentNames {
		if intentTree.GetTreeContext().NonRevertiveInfo().IsGenerallyNonRevertive(name) {
			hasNonRevertive = true
			break
		}
	}
	if !hasNonRevertive {
		d.coldStartLoaded = true
		return nil
	}
	if err := intentTree.FinishInsertionPhase(ctx); err != nil {
		return err
	}

	walkActiveIntendedWinners(intentTree.Entry, func(entry api.Entry, winning *api.LeafEntry) {
		if !intentTree.GetTreeContext().NonRevertiveInfo().IsNonRevertive(winning.Owner(), entry) {
			return
		}
		d.armInitialSyncSeed(winning, true)
	})
	d.coldStartLoaded = true
	return nil
}

// attachValidationCryptOverlay installs a validation-scoped learned-path overlay
// onto root's TreeContext for unannotated crypt leaves won by PreviouslyApplied
// Non-revertive intents and returns a restore callback that detaches it once
// validation finishes.
func (d *Datastore) attachValidationCryptOverlay(root *tree.RootEntry, newIntents map[string]*types.TransactionIntent) func() {
	noop := func() {}
	if d == nil || root == nil || root.GetTreeContext() == nil {
		return noop
	}
	eligibleOwners := eligiblePreviouslyAppliedOwners(newIntents)
	if len(eligibleOwners) == 0 {
		return noop
	}

	transient := make(map[keylessLeafPath]struct{})
	walkEligibleNonRevertiveWinners(root, eligibleOwners, func(entry api.Entry, winning *api.LeafEntry) {
		mode, ok := baselineSeedingMode(winning)
		if !ok || mode != modeCryptDiscovery {
			return
		}
		running := entry.GetLeafVariants().GetByOwner(consts.RunningIntentName)
		if running == nil || running.GetDeleteFlag() {
			return
		}
		if isCryptDiscoveryMatch(winning.Value(), running.Value()) {
			if kp := newKeylessLeafPath(entry.SdcpbPath()); kp != "" {
				transient[kp] = struct{}{}
			}
		}
	})
	if len(transient) == 0 {
		return noop
	}
	tc := root.GetTreeContext()
	tc.SetLearnedMutateOnWritePaths(learnedPathOverlay{
		base:      d.confirmedApplied,
		transient: transient,
	})
	return func() {
		tc.SetLearnedMutateOnWritePaths(d.confirmedApplied)
	}
}

// evictStaleBaselines removes baselines or pending markers whose owner and
// intendedValue are no longer the active winning intended leaf in root.
func (d *Datastore) evictStaleBaselines(ctx context.Context, root *tree.RootEntry) {
	if d == nil || d.confirmedApplied == nil || root == nil {
		return
	}
	for _, id := range d.confirmedApplied.SnapshotIdentities() {
		entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, id.path)
		if err != nil || entry == nil {
			d.confirmedApplied.DeleteIdentity(id)
			continue
		}
		winning := entry.GetLeafVariants().ActiveIntendedWinner()
		if winning == nil || !id.matches(winning.Owner(), winning.Value()) {
			d.confirmedApplied.DeleteIdentity(id)
		}
	}
}

// capturePreTransactionBaselines records pre-transaction baselines and southbound
// paths before a forward TransactionSet mutates confirmedApplied. Callers must hold dmutex.
func (d *Datastore) capturePreTransactionBaselines(updates api.LeafVariantSlice, deletes treetypes.DeleteEntriesList) {
	d.preTxBaselineSnap = d.confirmedApplied.Snapshot()
	d.preTxTouchedPaths = collectSouthboundLeafAndDeleteKeys(updates, deletes)
}

// restoreRolledBackBaselines restores untouched Non-revertive baselines and
// rolls back unconfirmed PreviouslyApplied path learning when a TransactionSet
// is canceled or expires. Callers must hold dmutex.
func (d *Datastore) restoreRolledBackBaselines(
	ctx context.Context,
	root *tree.RootEntry,
	rollbackUpdates api.LeafVariantSlice,
	rollbackDeletes treetypes.DeleteEntriesList,
) {
	snap := d.preTxBaselineSnap
	forwardTouched := d.preTxTouchedPaths
	d.preTxBaselineSnap = nil
	d.preTxTouchedPaths = nil
	if snap == nil || root == nil {
		return
	}

	rollbackTouched := collectSouthboundLeafAndDeleteKeys(rollbackUpdates, rollbackDeletes)
	deletePrefixes := deletesToPaths(rollbackDeletes)

	d.confirmedApplied.RestoreOnRollback(
		snap,
		func(path *sdcpb.Path) bool {
			key := newCanonicalLeafPath(path)
			if _, ok := forwardTouched[key]; ok {
				return true
			}
			if _, ok := rollbackTouched[key]; ok {
				return true
			}
			return pathCoveredBySync(path, deletePrefixes)
		},
		func(id baselineIdentity) bool {
			entry, err := ops.NavigateSdcpbPath(ctx, root.Entry, id.path)
			if err != nil || entry == nil {
				return false
			}
			winning := entry.GetLeafVariants().ActiveIntendedWinner()
			return winning != nil && id.matches(winning.Owner(), winning.Value())
		},
	)
}

func collectSouthboundLeafAndDeleteKeys(updates api.LeafVariantSlice, deletes treetypes.DeleteEntriesList) map[canonicalLeafPath]struct{} {
	if len(updates) == 0 && len(deletes) == 0 {
		return nil
	}
	keys := make(map[canonicalLeafPath]struct{}, len(updates)+len(deletes))
	for _, le := range updates {
		keys[newCanonicalLeafPath(le.SdcpbPath())] = struct{}{}
	}
	for _, del := range deletes {
		keys[newCanonicalLeafPath(del.SdcpbPath())] = struct{}{}
	}
	return keys
}

func deletesToPaths(deletes treetypes.DeleteEntriesList) []*sdcpb.Path {
	if len(deletes) == 0 {
		return nil
	}
	paths := make([]*sdcpb.Path, 0, len(deletes))
	for _, del := range deletes {
		paths = append(paths, del.SdcpbPath())
	}
	return paths
}

// seedPreviouslyAppliedBaselines seeds confirmed applied values from live
// Running (or defers seeding until initial sync when path has not yet synced) for
// unpushed leaves won by PreviouslyApplied Non-revertive intents. Acquires
// syncTreeMutex for reading; callers must hold dmutex and not syncTreeMutex.
func (d *Datastore) seedPreviouslyAppliedBaselines(
	root *tree.RootEntry,
	newIntents map[string]*types.TransactionIntent,
	pushedUpdates api.LeafVariantSlice,
) {
	if d == nil || d.confirmedApplied == nil || root == nil {
		return
	}
	eligibleOwners := eligiblePreviouslyAppliedOwners(newIntents)
	if len(eligibleOwners) == 0 {
		return
	}

	pushed := make(map[canonicalLeafPath]struct{}, len(pushedUpdates))
	for _, le := range pushedUpdates {
		pushed[newCanonicalLeafPath(le.SdcpbPath())] = struct{}{}
	}

	d.syncTreeMutex.RLock()
	defer d.syncTreeMutex.RUnlock()

	walkEligibleNonRevertiveWinners(root, eligibleOwners, func(entry api.Entry, winning *api.LeafEntry) {
		path := entry.SdcpbPath()
		if _, wasPushed := pushed[newCanonicalLeafPath(path)]; wasPushed {
			return
		}
		if runningVal := d.runningValueLocked(path); runningVal != nil {
			d.seedLeafBaselineFromRunning(winning, runningVal)
			return
		}
		if pathCoveredBySync(path, d.syncedPaths) {
			d.confirmedApplied.Delete(path)
			return
		}
		if _, ok := baselineSeedingMode(winning); !ok {
			d.confirmedApplied.Delete(path)
			return
		}
		d.armInitialSyncSeed(winning, false)
	})
}

func eligiblePreviouslyAppliedOwners(newIntents map[string]*types.TransactionIntent) map[string]struct{} {
	var eligible map[string]struct{}
	for _, intent := range newIntents {
		if !intent.GetDeleteFlag() && intent.NonRevertive() && intent.GetPreviouslyApplied() {
			if eligible == nil {
				eligible = make(map[string]struct{})
			}
			eligible[intent.GetName()] = struct{}{}
		}
	}
	return eligible
}

// baselineSeedingMode classifies winning from its static YANG schema rather
// than dynamically learned paths so unannotated string siblings still require
// crypt/cipher verification and pre-hashed plain strings never seed baselines.
func baselineSeedingMode(winning *api.LeafEntry) (baselineMode, bool) {
	if winning == nil || winning.GetEntry() == nil {
		return 0, false
	}
	if winning.GetEntry().GetSchema().IsMutateOnWrite() {
		return modeMutateOnWrite, true
	}
	if isPlainStringLeafEntry(winning) && !matchesCryptOrCipherValue(winning.Value()) {
		return modeCryptDiscovery, true
	}
	return 0, false
}

func leafBaselineIdentity(winning *api.LeafEntry) baselineIdentity {
	return baselineIdentity{
		path:          winning.SdcpbPath(),
		owner:         winning.Owner(),
		intendedValue: winning.Value(),
	}
}

// seedLeafBaselineFromRunning seeds winning's baseline from runningVal when
// winning qualifies for schema mutate-on-write or crypt discovery, evicting any
// prior baseline at winning's path otherwise.
func (d *Datastore) seedLeafBaselineFromRunning(winning *api.LeafEntry, runningVal *sdcpb.TypedValue) {
	id := leafBaselineIdentity(winning)
	mode, ok := baselineSeedingMode(winning)
	if !ok {
		d.confirmedApplied.Delete(id.path)
		return
	}
	if mode == modeMutateOnWrite {
		d.confirmedApplied.Record(id.path, id.owner, id.intendedValue, runningVal)
		return
	}
	d.confirmedApplied.RecordDiscovered(id, runningVal)
}

// armInitialSyncSeed registers winning for deferred baseline seeding on the
// first sync covering winning's path. When preserveExisting is true, an existing
// baseline or pending marker on that path is left intact.
func (d *Datastore) armInitialSyncSeed(winning *api.LeafEntry, preserveExisting bool) {
	mode, ok := baselineSeedingMode(winning)
	if !ok {
		return
	}
	d.confirmedApplied.MarkAwaitingInitialSync(leafBaselineIdentity(winning), mode, preserveExisting)
}

// runningValueLocked returns the live Running TypedValue at path, or nil when
// absent or marked deleted. Callers must hold syncTreeMutex (read or write).
func (d *Datastore) runningValueLocked(path *sdcpb.Path) *sdcpb.TypedValue {
	if d.syncTree == nil || d.syncTree.Entry == nil {
		return nil
	}
	syncEntry := ops.NavigateChildMap(d.syncTree.Entry, canonicalPath(path))
	if syncEntry == nil {
		return nil
	}
	running := syncEntry.GetLeafVariants().GetByOwner(consts.RunningIntentName)
	if running == nil || running.GetDeleteFlag() {
		return nil
	}
	return running.Value()
}

func walkEligibleNonRevertiveWinners(
	root *tree.RootEntry,
	eligibleOwners map[string]struct{},
	visit func(entry api.Entry, winning *api.LeafEntry),
) {
	tc := root.GetTreeContext()
	walkActiveIntendedWinners(root.Entry, func(entry api.Entry, winning *api.LeafEntry) {
		if _, ok := eligibleOwners[winning.Owner()]; !ok {
			return
		}
		if !tc.NonRevertiveInfo().IsNonRevertive(winning.Owner(), entry) {
			return
		}
		visit(entry, winning)
	})
}

func walkActiveIntendedWinners(entry api.Entry, visit func(entry api.Entry, winning *api.LeafEntry)) {
	if entry == nil {
		return
	}
	if winning := entry.GetLeafVariants().ActiveIntendedWinner(); winning != nil {
		visit(entry, winning)
	}
	for _, child := range entry.GetChilds(treetypes.DescendMethodActiveChilds) {
		walkActiveIntendedWinners(child, visit)
	}
}
