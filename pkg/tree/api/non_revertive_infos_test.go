package api

import (
	"testing"

	"github.com/sdcio/data-server/mocks/mocksdcpbpath"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestNewNonRevertiveInfos(t *testing.T) {
	n := NewNonRevertiveInfos()
	if n == nil {
		t.Fatal("expected non-nil NonRevertiveInfos")
	}
	if len(n) != 0 {
		t.Fatalf("expected empty map, got len=%d", len(n))
	}
}

func TestNonRevertiveInfos_Add(t *testing.T) {
	tests := []struct {
		name string
		adds []struct {
			owner        string
			nonRevertive bool
		}
		checkOwner       string
		wantNonRevertive bool
	}{
		{
			name: "adds new entry as non-revertive",
			adds: []struct {
				owner        string
				nonRevertive bool
			}{{"owner1", true}},
			checkOwner:       "owner1",
			wantNonRevertive: true,
		},
		{
			name: "does not overwrite existing entry",
			adds: []struct {
				owner        string
				nonRevertive bool
			}{
				{"owner1", true},
				{"owner1", false}, // should be ignored
			},
			checkOwner:       "owner1",
			wantNonRevertive: true,
		},
		{
			name: "adds new entry as revertive",
			adds: []struct {
				owner        string
				nonRevertive bool
			}{{"owner1", false}},
			checkOwner:       "owner1",
			wantNonRevertive: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNonRevertiveInfos()
			for _, a := range tt.adds {
				n.Add(a.owner, a.nonRevertive)
			}
			info, ok := n[tt.checkOwner]
			if !ok {
				t.Fatalf("expected %q to be present", tt.checkOwner)
			}
			if info.IsGenerallyNonRevertive() != tt.wantNonRevertive {
				t.Errorf("IsGenerallyNonRevertive() = %v, want %v", info.IsGenerallyNonRevertive(), tt.wantNonRevertive)
			}
		})
	}
}

func TestNonRevertiveInfos_AddNonRevertivePath(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(n NonRevertiveInfos)
		owner       string
		path        *sdcpb.Path
		wantPresent bool
	}{
		{
			name:        "creates entry if not exists",
			setup:       func(n NonRevertiveInfos) {},
			owner:       "owner1",
			path:        &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}},
			wantPresent: true,
		},
		{
			name: "appends path to existing entry",
			setup: func(n NonRevertiveInfos) {
				n.Add("owner1", false)
			},
			owner:       "owner1",
			path:        &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}},
			wantPresent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNonRevertiveInfos()
			tt.setup(n)
			n.AddNonRevertivePath(tt.owner, tt.path)
			_, ok := n[tt.owner]
			if ok != tt.wantPresent {
				t.Errorf("entry present = %v, want %v", ok, tt.wantPresent)
			}
		})
	}
}

func TestNonRevertiveInfos_IsNonRevertive(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(n NonRevertiveInfos)
		owner      string
		pathElems  []*sdcpb.PathElem
		wantResult bool
	}{
		{
			name:       "unknown owner returns false",
			setup:      func(n NonRevertiveInfos) {},
			owner:      "unknown",
			pathElems:  []*sdcpb.PathElem{{Name: "a"}},
			wantResult: false,
		},
		{
			name:       "generally non-revertive owner with no paths",
			setup:      func(n NonRevertiveInfos) { n.Add("owner1", true) },
			owner:      "owner1",
			pathElems:  []*sdcpb.PathElem{{Name: "a"}},
			wantResult: true,
		},
		{
			name:       "generally revertive owner with no paths",
			setup:      func(n NonRevertiveInfos) { n.Add("owner1", false) },
			owner:      "owner1",
			pathElems:  []*sdcpb.PathElem{{Name: "a"}},
			wantResult: false,
		},
		{
			name: "path outside revert paths is non-revertive",
			setup: func(n NonRevertiveInfos) {
				n.AddNonRevertivePath("owner1", &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}})
			},
			owner:      "owner1",
			pathElems:  []*sdcpb.PathElem{{Name: "bgp"}},
			wantResult: true,
		},
		{
			name: "path under revert path is revertive",
			setup: func(n NonRevertiveInfos) {
				n.AddNonRevertivePath("owner1", &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "interface"}}})
			},
			owner:      "owner1",
			pathElems:  []*sdcpb.PathElem{{Name: "interface"}, {Name: "eth0"}},
			wantResult: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNonRevertiveInfos()
			tt.setup(n)
			ctrl := gomock.NewController(t)
			mockPath := mocksdcpbpath.NewMockSdcpbPath(ctrl)
			mockPath.EXPECT().SdcpbPath().Return(&sdcpb.Path{Elem: tt.pathElems}).AnyTimes()
			got := n.IsNonRevertive(tt.owner, mockPath)
			if got != tt.wantResult {
				t.Errorf("IsNonRevertive() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestNonRevertiveInfos_IsGenerallyNonRevertive(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(n NonRevertiveInfos)
		owner      string
		wantResult bool
	}{
		{
			name:       "unknown owner returns false",
			setup:      func(n NonRevertiveInfos) {},
			owner:      "unknown",
			wantResult: false,
		},
		{
			name:       "true for generally non-revertive owner",
			setup:      func(n NonRevertiveInfos) { n.Add("owner1", true) },
			owner:      "owner1",
			wantResult: true,
		},
		{
			name:       "false for generally revertive owner",
			setup:      func(n NonRevertiveInfos) { n.Add("owner1", false) },
			owner:      "owner1",
			wantResult: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNonRevertiveInfos()
			tt.setup(n)
			got := n.IsGenerallyNonRevertive(tt.owner)
			if got != tt.wantResult {
				t.Errorf("IsGenerallyNonRevertive() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestNonRevertiveInfos_DeepCopy(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(n NonRevertiveInfos)
		wantLen    int
		mutateKey  string
		wantInCopy bool
	}{
		{
			name: "copy is independent from original",
			setup: func(n NonRevertiveInfos) {
				n.Add("owner1", true)
				n.Add("owner2", false)
			},
			wantLen:    2,
			mutateKey:  "owner3",
			wantInCopy: false,
		},
		{
			name:       "empty map deep copy",
			setup:      func(n NonRevertiveInfos) {},
			wantLen:    0,
			mutateKey:  "owner1",
			wantInCopy: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNonRevertiveInfos()
			tt.setup(n)
			cp := n.DeepCopy()
			if cp == nil {
				t.Fatal("expected non-nil copy")
			}
			if len(cp) != tt.wantLen {
				t.Fatalf("copy length = %d, want %d", len(cp), tt.wantLen)
			}
			n.Add(tt.mutateKey, true)
			_, ok := cp[tt.mutateKey]
			if ok != tt.wantInCopy {
				t.Errorf("key %q in copy = %v, want %v", tt.mutateKey, ok, tt.wantInCopy)
			}
		})
	}
}
