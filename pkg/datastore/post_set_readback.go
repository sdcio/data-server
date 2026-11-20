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
	"strings"

	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	treetypes "github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

func (d *Datastore) isNoopTarget() bool {
	return d.config != nil && d.config.SBI != nil && strings.EqualFold(d.config.SBI.Type, "noop")
}

func isMutateOnWriteLeafEntry(le *api.LeafEntry) bool {
	return le != nil && le.GetEntry() != nil && le.GetEntry().GetSchema().IsMutateOnWrite()
}

func collectPushedMutateOnWritePaths(updates api.LeafVariantSlice) []*sdcpb.Path {
	var paths []*sdcpb.Path
	seen := make(map[canonicalLeafPath]struct{})
	for _, le := range updates {
		if !isMutateOnWriteLeafEntry(le) || le.GetDeleteFlag() || le.GetExplicitDeleteFlag() {
			continue
		}
		p := le.SdcpbPath()
		k := newCanonicalLeafPath(p)
		if _, exists := seen[k]; !exists {
			seen[k] = struct{}{}
			paths = append(paths, canonicalPath(p))
		}
	}
	return paths
}

// fetchMutateOnWriteReadbacks issues a single targeted CONFIG Get for paths on
// real targets and returns non-empty schema-typed leaf values keyed by canonicalLeafPath.
// Callers must hold dmutex.
func (d *Datastore) fetchMutateOnWriteReadbacks(ctx context.Context, paths []*sdcpb.Path) map[canonicalLeafPath]*sdcpb.TypedValue {
	if len(paths) == 0 || d.isNoopTarget() {
		return nil
	}
	sbi := d.getSBI()
	if sbi == nil {
		return nil
	}

	req := &sdcpb.GetDataRequest{
		Path:     paths,
		DataType: sdcpb.DataType_CONFIG,
	}
	if d.config != nil {
		req.Name = d.config.Name
		if enc, ok := d.gnmiReadbackEncoding(); ok {
			req.Encoding = enc
		}
	}

	log := logger.FromContext(ctx)
	rsp, err := sbi.Get(ctx, req)
	if err != nil {
		log.Error(err, "post-Set targeted Get for mutate-on-write leaves failed", "severity", "WARN")
		return nil
	}

	converter := utils.NewConverter(d.schemaClient)
	readbacks := make(map[canonicalLeafPath]*sdcpb.TypedValue, len(paths))
	for _, noti := range rsp.GetNotification() {
		for _, rawUpd := range noti.GetUpdate() {
			expanded, expErr := converter.ExpandUpdate(ctx, proto.Clone(rawUpd).(*sdcpb.Update))
			if expErr != nil {
				log.Error(expErr, "failed expanding post-Set mutate-on-write readback", "severity", "WARN")
				continue
			}
			for _, upd := range expanded {
				if !isNonEmptyTypedValue(upd.GetValue()) {
					continue
				}
				readbacks[newCanonicalLeafPath(upd.GetPath())] = upd.GetValue()
			}
		}
	}
	for _, path := range paths {
		if _, ok := readbacks[newCanonicalLeafPath(path)]; !ok {
			log.Info("post-Set targeted Get omitted mutate-on-write leaf", "severity", "WARN", "path", path.ToXPath(false))
		}
	}
	return readbacks
}

func (d *Datastore) gnmiReadbackEncoding() (sdcpb.Encoding, bool) {
	if d.config == nil || d.config.SBI == nil || d.config.SBI.GnmiOptions == nil || d.config.SBI.GnmiOptions.Encoding == "" {
		return 0, false
	}
	enc, err := sdcpb.ParseEncoding(d.config.SBI.GnmiOptions.Encoding)
	if err != nil {
		return 0, false
	}
	return enc, true
}

// applyReadbacksAndWriteBackRunning binds non-empty post-Set readbacks and
// leaves missing values pending for causal sync capture while updating Running.
func (d *Datastore) applyReadbacksAndWriteBackRunning(ctx context.Context, updates api.LeafVariantSlice, deletes treetypes.DeleteEntriesList, readbacks map[canonicalLeafPath]*sdcpb.TypedValue, setDoneUnixNano int64) error {
	return d.writeReadbacksToRunning(ctx, updates, deletes, readbacks, func(le *api.LeafEntry, rb *sdcpb.TypedValue, ok bool) {
		if ok {
			d.confirmedApplied.RecordAfter(le.SdcpbPath(), le.Owner(), le.Value(), rb, setDoneUnixNano)
		} else {
			d.confirmedApplied.MarkPending(le.SdcpbPath(), le.Owner(), le.Value(), setDoneUnixNano)
		}
	})
}

// reconcileReplaceReadbacksAndWriteBackRunning clears prior baselines after a
// full-config replace, binds non-empty readbacks to matching winning persisted
// owners when present, and writes confirmed applied values into Running.
func (d *Datastore) reconcileReplaceReadbacksAndWriteBackRunning(ctx context.Context, updates api.LeafVariantSlice, deletes treetypes.DeleteEntriesList, readbacks map[canonicalLeafPath]*sdcpb.TypedValue, setDoneUnixNano int64) error {
	d.confirmedApplied.ClearAll()

	var persistedIntents *tree.RootEntry
	if len(collectPushedMutateOnWritePaths(updates)) > 0 {
		if root, err := d.NewEmptyTree(ctx); err == nil {
			if _, loadErr := d.LoadAllButRunningIntents(ctx, root); loadErr == nil && root.FinishInsertionPhase(ctx) == nil {
				persistedIntents = root
			}
		}
	}

	return d.writeReadbacksToRunning(ctx, updates, deletes, readbacks, func(le *api.LeafEntry, rb *sdcpb.TypedValue, ok bool) {
		owner, matched := winningPersistedOwnerWithValue(ctx, persistedIntents, le.SdcpbPath(), le.Value())
		if !matched {
			return
		}
		if ok {
			d.confirmedApplied.RecordAfter(le.SdcpbPath(), owner, le.Value(), rb, setDoneUnixNano)
		} else {
			d.confirmedApplied.MarkPending(le.SdcpbPath(), owner, le.Value(), setDoneUnixNano)
		}
	})
}

func (d *Datastore) writeReadbacksToRunning(
	ctx context.Context,
	updates api.LeafVariantSlice,
	deletes treetypes.DeleteEntriesList,
	readbacks map[canonicalLeafPath]*sdcpb.TypedValue,
	onMutateOnWrite func(le *api.LeafEntry, rb *sdcpb.TypedValue, hasReadback bool),
) error {
	runningUpdates := make([]*treetypes.PathAndUpdate, 0, len(updates))
	for _, le := range updates {
		runVal := le.Value()
		if isMutateOnWriteLeafEntry(le) {
			rb, ok := readbacks[newCanonicalLeafPath(le.SdcpbPath())]
			onMutateOnWrite(le, rb, ok)
			if ok {
				runVal = proto.Clone(rb).(*sdcpb.TypedValue)
			}
		}
		runningUpdates = append(runningUpdates, treetypes.NewPathAndUpdate(
			le.SdcpbPath(),
			treetypes.NewUpdate(nil, runVal, consts.RunningValuesPrio, consts.RunningIntentName, le.Timestamp()),
		))
	}
	return d.writeBackRunningUpdates(ctx, runningUpdates, deletes)
}

func winningPersistedOwnerWithValue(ctx context.Context, persistedIntents *tree.RootEntry, path *sdcpb.Path, val *sdcpb.TypedValue) (string, bool) {
	if persistedIntents == nil {
		return "", false
	}
	entry, err := ops.NavigateSdcpbPath(ctx, persistedIntents.Entry, path)
	if err != nil || entry == nil {
		return "", false
	}
	highest := entry.GetLeafVariants().GetHighestPrecedence(false, false, false)
	if highest == nil || !api.TypedValuesEqual(highest.Value(), val) {
		return "", false
	}
	return highest.Owner(), true
}
