package tree

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdcio/data-server/mocks/mocksdcpbpath"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree/interfaces"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

// newMockSdcpbPath creates a new mock SdcpbPath with the given PathElems
func newMockSdcpbPath(ctrl *gomock.Controller, elem []*sdcpb.PathElem) *mocksdcpbpath.MockSdcpbPath {
	mock := mocksdcpbpath.NewMockSdcpbPath(ctrl)
	path := &sdcpb.Path{
		Elem:        elem,
		IsRootBased: false,
	}
	mock.EXPECT().SdcpbPath().Return(path).AnyTimes()
	return mock
}

func Test_TreeContext_AddNonRevertiveInfo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.TODO(), runtime.GOMAXPROCS(0)))

	tests := []struct {
		name         string
		intent       string
		nonRevertive bool
	}{
		{
			name:         "Add non-revertive intent",
			intent:       "intent-1",
			nonRevertive: true,
		},
		{
			name:         "Add revertive intent",
			intent:       "intent-2",
			nonRevertive: false,
		},
		{
			name:         "Add multiple intents",
			intent:       "intent-3",
			nonRevertive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc.AddNonRevertiveInfo(tt.intent, tt.nonRevertive)

			if _, ok := tc.nonRevertiveInfo[tt.intent]; !ok {
				t.Errorf("expected intent %s to be added", tt.intent)
			}

			info := tc.nonRevertiveInfo[tt.intent]
			if info.GetGeneralNonRevertiveState() != tt.nonRevertive {
				t.Errorf("expected nonRevertive=%v, got %v", tt.nonRevertive, info.GetGeneralNonRevertiveState())
			}
		})
	}
}

func Test_TreeContext_IsGenerallyNonRevertiveIntent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.TODO(), runtime.GOMAXPROCS(0)))

	tests := []struct {
		name         string
		intent       string
		nonRevertive bool
		shouldAdd    bool
		expected     bool
	}{
		{
			name:         "Non-existent intent returns false",
			intent:       "non-existent",
			nonRevertive: false,
			shouldAdd:    false,
			expected:     false,
		},
		{
			name:         "Non-revertive intent returns true",
			intent:       "intent-non-revertive",
			nonRevertive: true,
			shouldAdd:    true,
			expected:     true,
		},
		{
			name:         "Revertive intent returns false",
			intent:       "intent-revertive",
			nonRevertive: false,
			shouldAdd:    true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldAdd {
				tc.AddNonRevertiveInfo(tt.intent, tt.nonRevertive)
			}

			result := tc.IsGenerallyNonRevertiveIntent(tt.intent)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func Test_TreeContext_IsNonRevertiveIntentPath(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.TODO(), runtime.GOMAXPROCS(0)))

	// Create test paths
	parentPath := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("interface", map[string]string{"name": "eth-1"}),
	})

	sameParentPath := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("interface", map[string]string{"name": "eth-1"}),
	})

	unrelatedPath := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("routing", nil),
	})

	tests := []struct {
		name         string
		intent       string
		nonRevertive bool
		shouldAdd    bool
		pathsToAdd   []*sdcpb.Path
		queryPath    interfaces.SdcpbPath
		expected     bool
		description  string
	}{
		{
			name:         "Non-existent intent returns false",
			intent:       "non-existent",
			nonRevertive: false,
			shouldAdd:    false,
			queryPath:    parentPath,
			expected:     false,
			description:  "should return false for non-existent intent",
		},
		{
			name:         "Revertive intent with no paths, returns false",
			intent:       "intent-1",
			nonRevertive: false,
			shouldAdd:    true,
			pathsToAdd:   nil,
			queryPath:    parentPath,
			expected:     false,
			description:  "should return false when intent has general nonRevertive=false and no specific paths",
		},
		{
			name:         "Non-revertive intent with no paths, returns true",
			intent:       "intent-2",
			nonRevertive: true,
			shouldAdd:    true,
			pathsToAdd:   nil,
			queryPath:    parentPath,
			expected:     true,
			description:  "should return true when intent has general nonRevertive=true and no specific paths",
		},
		{
			name:         "Intent with exact path match returns false",
			intent:       "intent-3",
			nonRevertive: false,
			shouldAdd:    true,
			pathsToAdd:   []*sdcpb.Path{parentPath.SdcpbPath()},
			queryPath:    sameParentPath,
			expected:     false,
			description:  "should return true when query path matches a path in revertPaths (path is parent of itself)",
		},
		{
			name:         "Intent with no matching path returns true",
			intent:       "intent-4",
			nonRevertive: false,
			shouldAdd:    true,
			pathsToAdd:   []*sdcpb.Path{parentPath.SdcpbPath()},
			queryPath:    unrelatedPath,
			expected:     true,
			description:  "should return false when query path doesn't match any configured revert paths",
		},
		{
			name:         "Non-revertive intent with explicit paths uses path matching",
			intent:       "intent-5",
			nonRevertive: true,
			shouldAdd:    true,
			pathsToAdd:   []*sdcpb.Path{unrelatedPath.SdcpbPath()},
			queryPath:    parentPath,
			expected:     true,
			description:  "when specific paths are set, they override the general nonRevertive state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldAdd {
				tc.AddNonRevertiveInfo(tt.intent, tt.nonRevertive)

				if len(tt.pathsToAdd) > 0 {
					info := tc.nonRevertiveInfo[tt.intent]
					for _, path := range tt.pathsToAdd {
						info.AddPath(path)
					}
				}
			}

			result := tc.IsNonRevertiveIntentPath(tt.intent, tt.queryPath)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expected, result)
			}
		})
	}
}

func Test_NonRevertiveInfo_DeepCopy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.TODO(), runtime.GOMAXPROCS(0)))

	path := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("interface", map[string]string{"name": "eth-1"}),
	})

	tc.AddNonRevertiveInfo("intent-1", true)
	info := tc.nonRevertiveInfo["intent-1"]
	info.AddPath(path.SdcpbPath())

	// Deep copy
	copied := info.DeepCopy()

	// Verify copy has same values
	if copied.GetGeneralNonRevertiveState() != info.GetGeneralNonRevertiveState() {
		t.Errorf("copied nonRevertive state differs from original")
	}

	if len(copied.revertPaths) != len(info.revertPaths) {
		t.Errorf("copied paths count differs from original: expected %d, got %d",
			len(info.revertPaths), len(copied.revertPaths))
	}

	// Verify it's a deep copy by modifying and checking independence
	tc.AddNonRevertiveInfo("intent-2", false)
	info2 := tc.nonRevertiveInfo["intent-2"]
	info2Path := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("routing", nil),
	})
	info2.AddPath(info2Path.SdcpbPath())

	_ = info2.DeepCopy()

	// Verify they are independent
	if len(copied.revertPaths) != 1 {
		t.Errorf("original copy should not be affected by new additions: expected 1 path, got %d",
			len(copied.revertPaths))
	}
}

func Test_TreeContext_deepCopy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	scb, err := testhelper.GetSchemaClientBound(t, mockCtrl)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(scb, pool.NewSharedTaskPool(context.TODO(), runtime.GOMAXPROCS(0)))

	path1 := newMockSdcpbPath(mockCtrl, []*sdcpb.PathElem{
		sdcpb.NewPathElem("interface", map[string]string{"name": "eth-1"}),
	})

	tc.AddNonRevertiveInfo("intent-1", true)
	tc.nonRevertiveInfo["intent-1"].AddPath(path1.SdcpbPath())

	// Deep copy the TreeContext
	copied := tc.deepCopy()

	// Verify copy has the same intent
	if _, ok := copied.nonRevertiveInfo["intent-1"]; !ok {
		t.Errorf("copied TreeContext should have intent-1")
	}

	// Verify the copy is independent
	tc.AddNonRevertiveInfo("intent-2", false)

	if _, ok := copied.nonRevertiveInfo["intent-2"]; ok {
		t.Errorf("copied TreeContext should not have intent-2")
	}

	// Verify they have the same schema client and pool factory
	if copied.schemaClient != tc.schemaClient {
		t.Errorf("schema client should be the same reference")
	}

	if copied.poolFactory != tc.poolFactory {
		t.Errorf("pool factory should be the same reference")
	}
}
