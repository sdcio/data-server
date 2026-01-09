package tree

import (
	"testing"

	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type mockUpdateParent struct{}

func (m *mockUpdateParent) SdcpbPath() *sdcpb.Path {
	return &sdcpb.Path{}
}

func TestLeafVariants_remainsToExist(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *LeafVariants
		expected bool
	}{
		{
			name: "Empty LeafVariants",
			setup: func() *LeafVariants {
				return &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
			},
			expected: false,
		},
		{
			name: "Single entry, not deleted",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				le := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner1", 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(le)
				return lv
			},
			expected: true,
		},
		{
			name: "Single entry, deleted",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				le := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				lv.Add(le)
				return lv
			},
			expected: false,
		},
		{
			name: "Multiple entries, all deleted",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 20, "owner2", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				return lv
			},
			expected: false,
		},
		{
			name: "Multiple entries, one remaining",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 20, "owner2", 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				return lv
			},
			expected: true,
		},
		{
			name: "Explicit delete highest priority",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				// Explicit delete with priority 5 (lower is higher priority)
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 5, "owner1", 0),
					types.NewUpdateInsertFlags().SetExplicitDeleteFlag(),
					nil,
				)
				// Normal entry with priority 10
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner2", 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				return lv
			},
			expected: false,
		},
		{
			name: "Explicit delete lower priority",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				// Explicit delete with priority 20
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 20, "owner1", 0),
					types.NewUpdateInsertFlags().SetExplicitDeleteFlag(),
					nil,
				)
				// Normal entry with priority 10 (higher priority)
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner2", 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				return lv
			},
			expected: true,
		},

		{
			name: "Delete all, no running",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				// Explicit delete with priority 20
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 20, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				// Normal entry with priority 10 (higher priority)
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner2", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				return lv
			},
			expected: false,
		},
		{
			name: "Delete all, with running",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				// Explicit delete with priority 20
				le1 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 20, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				// Normal entry with priority 10 (higher priority)
				le2 := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner2", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				lerun := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, RunningValuesPrio, RunningIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(le1)
				lv.Add(le2)
				lv.Add(lerun)
				return lv
			},
			expected: false,
		},
		{
			name: "Only Running",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				lerun := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, RunningValuesPrio, RunningIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(lerun)
				return lv
			},
			expected: true,
		},
		{
			name: "Only Running + default",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				lerun := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, RunningValuesPrio, RunningIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				ledef := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, DefaultValuesPrio, DefaultsIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				lv.Add(lerun)
				lv.Add(ledef)
				return lv
			},
			expected: true,
		},
		{
			name: "Running, default and delete",
			setup: func() *LeafVariants {
				lv := &LeafVariants{
					les: make(LeafVariantSlice, 0),
				}
				lerun := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, RunningValuesPrio, RunningIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				ledef := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, DefaultValuesPrio, DefaultsIntentName, 0),
					types.NewUpdateInsertFlags(),
					nil,
				)
				ledel := NewLeafEntry(
					types.NewUpdate(&mockUpdateParent{}, &sdcpb.TypedValue{}, 10, "owner1", 0),
					types.NewUpdateInsertFlags().SetDeleteFlag(),
					nil,
				)
				lv.Add(lerun)
				lv.Add(ledef)
				lv.Add(ledel)
				return lv
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := tt.setup()
			if got := lv.remainsToExist(); got != tt.expected {
				t.Errorf("LeafVariants.remainsToExist() = %v, want %v", got, tt.expected)
			}
		})
	}
}
