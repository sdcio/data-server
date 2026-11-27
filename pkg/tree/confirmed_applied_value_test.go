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

package tree_test

import (
	"context"
	"runtime"
	"testing"

	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/consts"
	"github.com/sdcio/data-server/pkg/tree/ops"
	"github.com/sdcio/data-server/pkg/tree/processors"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type stubBaselineResolver struct {
	path      string
	owner     string
	intended  *sdcpb.TypedValue
	confirmed *sdcpb.TypedValue
}

func (s *stubBaselineResolver) Lookup(path *sdcpb.Path, owner string, intendedValue *sdcpb.TypedValue) *sdcpb.TypedValue {
	if s == nil || path == nil || intendedValue == nil {
		return nil
	}
	if path.ToXPath(false) == s.path && owner == s.owner && intendedValue.Equal(s.intended) {
		return s.confirmed
	}
	return nil
}

func stringTV(v string) *sdcpb.TypedValue {
	return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
}

func TestTree_ConfirmedAppliedValueBaseline(t *testing.T) {
	ctx := context.Background()
	sc, schemaCfg, err := testhelper.InitSDCIOSchema()
	if err != nil {
		t.Fatalf("InitSDCIOSchema: %v", err)
	}
	scb := schemaClient.NewSchemaClientBound(schemaCfg, sc)
	vpf := pool.NewSharedTaskPool(ctx, runtime.GOMAXPROCS(0))

	leafPath := &sdcpb.Path{
		IsRootBased: true,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
			sdcpb.NewPathElem("description", nil),
		},
	}
	keyPath := &sdcpb.Path{
		IsRootBased: true,
		Elem: []*sdcpb.PathElem{
			sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
			sdcpb.NewPathElem("name", nil),
		},
	}

	authored := stringTV("cleartext-secret")
	confirmed := stringTV("$6$rounds=5000$salt$hashed")
	diverged := stringTV("$6$rounds=5000$other$tampered")

	t.Run("nil-safe TreeContext without resolver falls back to literal intended value", func(t *testing.T) {
		var nilTC *tree.TreeContext
		if got := nilTC.ConfirmedAppliedValue(leafPath, "intent1", authored); got != nil {
			t.Fatalf("nil TreeContext returned %v, want nil", got)
		}
		zeroTC := &tree.TreeContext{}
		if got := zeroTC.ConfirmedAppliedValue(leafPath, "intent1", authored); got != nil {
			t.Fatalf("zero TreeContext returned %v, want nil", got)
		}

		tc := tree.NewTreeContext(scb, vpf)
		root, err := tree.NewTreeRoot(ctx, tc)
		if err != nil {
			t.Fatalf("NewTreeRoot: %v", err)
		}
		err = root.AddUpdatesRecursive(ctx, []*types.PathAndUpdate{
			types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), 10, "intent1", 0)),
			types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, authored, 10, "intent1", 0)),
			types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), consts.RunningValuesPrio, consts.RunningIntentName, 0)),
			types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, confirmed, consts.RunningValuesPrio, consts.RunningIntentName, 0)),
		}, types.NewUpdateInsertFlags())
		if err != nil {
			t.Fatalf("AddUpdatesRecursive: %v", err)
		}
		if err := root.FinishInsertionPhase(ctx); err != nil {
			t.Fatalf("FinishInsertionPhase: %v", err)
		}

		upds := root.GetHighestPrecedence(true)
		if len(upds) != 1 || !upds[0].Value().Equal(authored) {
			t.Fatalf("expected 1 update with authored value when no baseline resolver is set, got %v", upds)
		}
	})

	t.Run("converged baseline suppresses diff, NOT_APPLIED deviation, and BlameConfig deviation while preserving authored values", func(t *testing.T) {
		var resolver api.ConfirmedAppliedValueResolver = &stubBaselineResolver{
			path:      "/interface[name=ethernet-1/1]/description",
			owner:     "intent1",
			intended:  authored,
			confirmed: confirmed,
		}
		tc := tree.NewTreeContext(scb, vpf, tree.WithConfirmedAppliedValues(resolver))
		origRoot, err := tree.NewTreeRoot(ctx, tc)
		if err != nil {
			t.Fatalf("NewTreeRoot: %v", err)
		}
		// DeepCopy must preserve the wired baseline resolver.
		root, err := origRoot.DeepCopy(ctx)
		if err != nil {
			t.Fatalf("DeepCopy: %v", err)
		}

		overruledVal := stringTV("secondary-cleartext")
		err = root.AddUpdatesRecursive(ctx, []*types.PathAndUpdate{
			types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), 10, "intent1", 0)),
			types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, authored, 10, "intent1", 0)),
			types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), 20, "intent2", 0)),
			types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, overruledVal, 20, "intent2", 0)),
			types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), consts.RunningValuesPrio, consts.RunningIntentName, 0)),
			types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, confirmed, consts.RunningValuesPrio, consts.RunningIntentName, 0)),
		}, types.NewUpdateInsertFlags())
		if err != nil {
			t.Fatalf("AddUpdatesRecursive: %v", err)
		}
		if err := root.FinishInsertionPhase(ctx); err != nil {
			t.Fatalf("FinishInsertionPhase: %v", err)
		}

		// 1. Diffing: Running == confirmed -> zero new/updated entries.
		if upds := root.GetHighestPrecedence(true); len(upds) != 0 {
			t.Fatalf("expected 0 updates when Running matches confirmed baseline, got %v", upds)
		}

		// 2. Deviations: zero NOT_APPLIED; OVERRULED still reports authored values.
		devCh := make(chan *types.DeviationEntry, 16)
		if err := ops.GetDeviations(ctx, root.Entry, &ops.GetDeviationParams{Ch: devCh}, vpf); err != nil {
			t.Fatalf("GetDeviations: %v", err)
		}
		close(devCh)

		var devs []*types.DeviationEntry
		for d := range devCh {
			devs = append(devs, d)
		}
		if len(devs) != 1 {
			t.Fatalf("expected exactly 1 OVERRULED deviation and 0 NOT_APPLIED, got %d: %+v", len(devs), devs)
		}
		if devs[0].Reason() != types.DeviationReasonOverruled || devs[0].IntentName() != "intent2" {
			t.Fatalf("unexpected deviation: reason=%v intent=%s", devs[0].Reason(), devs[0].IntentName())
		}
		if !devs[0].ExpectedValue().Equal(overruledVal) || !devs[0].CurrentValue().Equal(authored) {
			t.Fatalf("OVERRULED values = (expected=%v, current=%v), want (%v, %v)",
				devs[0].ExpectedValue(), devs[0].CurrentValue(), overruledVal, authored)
		}

		// 3. BlameConfig: Value is winning owner's authored value, DeviationValue is nil.
		bp := processors.NewBlameConfigProcessor(&processors.BlameConfigProcessorParams{})
		blame, err := bp.Run(ctx, root.Entry, vpf)
		if err != nil {
			t.Fatalf("BlameConfig: %v", err)
		}
		descBlame := testhelper.FindBlameElement(blame, "interface", "ethernet-1/1", "description")
		if descBlame == nil {
			t.Fatalf("missing description blame node in %v", blame)
		}
		if descBlame.GetOwner() != "intent1" || !descBlame.GetValue().Equal(authored) {
			t.Fatalf("BlameConfig owner/value = (%q, %v), want (%q, %v)",
				descBlame.GetOwner(), descBlame.GetValue(), "intent1", authored)
		}
		if descBlame.GetDeviationValue() != nil {
			t.Fatalf("expected nil DeviationValue when Running matches confirmed baseline, got %v", descBlame.GetDeviationValue())
		}
	})

	t.Run("diverged Running and nil Running still trigger diff, NOT_APPLIED with authored ExpectedValue, and BlameConfig deviation", func(t *testing.T) {
		resolver := &stubBaselineResolver{
			path:      "/interface[name=ethernet-1/1]/description",
			owner:     "intent1",
			intended:  authored,
			confirmed: confirmed,
		}

		for _, tcCase := range []struct {
			name       string
			runningVal *sdcpb.TypedValue
		}{
			{name: "Running differs from confirmed baseline", runningVal: diverged},
			{name: "Running is nil despite confirmed baseline", runningVal: nil},
		} {
			t.Run(tcCase.name, func(t *testing.T) {
				tc := tree.NewTreeContext(scb, vpf, tree.WithConfirmedAppliedValues(resolver))
				root, err := tree.NewTreeRoot(ctx, tc)
				if err != nil {
					t.Fatalf("NewTreeRoot: %v", err)
				}
				paus := []*types.PathAndUpdate{
					types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), 10, "intent1", 0)),
					types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, authored, 10, "intent1", 0)),
					types.NewPathAndUpdate(keyPath, types.NewUpdate(nil, stringTV("ethernet-1/1"), consts.RunningValuesPrio, consts.RunningIntentName, 0)),
				}
				if tcCase.runningVal != nil {
					paus = append(paus, types.NewPathAndUpdate(leafPath, types.NewUpdate(nil, tcCase.runningVal, consts.RunningValuesPrio, consts.RunningIntentName, 0)))
				}
				if err := root.AddUpdatesRecursive(ctx, paus, types.NewUpdateInsertFlags()); err != nil {
					t.Fatalf("AddUpdatesRecursive: %v", err)
				}
				if err := root.FinishInsertionPhase(ctx); err != nil {
					t.Fatalf("FinishInsertionPhase: %v", err)
				}

				upds := root.GetHighestPrecedence(true)
				if len(upds) != 1 || !upds[0].Value().Equal(authored) {
					t.Fatalf("expected 1 update with authored value, got %v", upds)
				}

				devCh := make(chan *types.DeviationEntry, 8)
				if err := ops.GetDeviations(ctx, root.Entry, &ops.GetDeviationParams{Ch: devCh}, vpf); err != nil {
					t.Fatalf("GetDeviations: %v", err)
				}
				close(devCh)

				var devs []*types.DeviationEntry
				for d := range devCh {
					devs = append(devs, d)
				}
				if len(devs) != 1 || devs[0].Reason() != types.DeviationReasonNotApplied {
					t.Fatalf("expected 1 NOT_APPLIED deviation, got %+v", devs)
				}
				if !devs[0].ExpectedValue().Equal(authored) {
					t.Fatalf("NOT_APPLIED ExpectedValue = %v, want authored %v", devs[0].ExpectedValue(), authored)
				}
				if tcCase.runningVal == nil && devs[0].CurrentValue() != nil {
					t.Fatalf("expected nil CurrentValue when Running is nil, got %v", devs[0].CurrentValue())
				}
				if tcCase.runningVal != nil && !devs[0].CurrentValue().Equal(tcCase.runningVal) {
					t.Fatalf("NOT_APPLIED CurrentValue = %v, want %v", devs[0].CurrentValue(), tcCase.runningVal)
				}

				bp := processors.NewBlameConfigProcessor(&processors.BlameConfigProcessorParams{})
				blame, err := bp.Run(ctx, root.Entry, vpf)
				if err != nil {
					t.Fatalf("BlameConfig: %v", err)
				}
				descBlame := testhelper.FindBlameElement(blame, "interface", "ethernet-1/1", "description")
				if descBlame == nil || !descBlame.GetValue().Equal(authored) {
					t.Fatalf("BlameConfig Value = %v, want authored %v", descBlame, authored)
				}
				if descBlame.GetDeviationValue() == nil {
					t.Fatalf("expected non-nil DeviationValue on %s", tcCase.name)
				}
			})
		}
	})
}
