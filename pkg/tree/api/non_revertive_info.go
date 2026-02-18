package api

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type NonRevertiveInfo struct {
	intentName   string
	nonRevertive bool
	revertPaths  sdcpb.Paths
}

func NewNonRevertiveInfo(intentName string, nonRevertive bool) *NonRevertiveInfo {
	return &NonRevertiveInfo{
		intentName:   intentName,
		nonRevertive: nonRevertive,
		revertPaths:  sdcpb.Paths{},
	}
}

func (n *NonRevertiveInfo) AddPath(path *sdcpb.Path) {
	n.revertPaths = append(n.revertPaths, path)
}

// IsGenerallyNonRevertive returns the general non-revertive state of the intent, which is true if the intent is non-revertive for all paths, false otherwise.
func (n *NonRevertiveInfo) IsGenerallyNonRevertive() bool {
	return n.nonRevertive
}

// IsNonRevertive returns true if the intent is non-revertive for the given path, false otherwise.
// If no paths are set for the intent, the general non-revertive state is returned.
func (n *NonRevertiveInfo) IsNonRevertive(p SdcpbPath) bool {
	if len(n.revertPaths) == 0 {
		return n.nonRevertive
	}

	// checks if any path in the Paths slice is a parent path of the given path. If such a path is found, return true else false.
	return !n.revertPaths.ContainsParentPath(p.SdcpbPath())
}

func (n *NonRevertiveInfo) DeepCopy() *NonRevertiveInfo {
	copy := &NonRevertiveInfo{
		intentName:   n.intentName,
		nonRevertive: n.nonRevertive,
		revertPaths:  make(sdcpb.Paths, len(n.revertPaths)),
	}
	copy.revertPaths = n.revertPaths.DeepCopy()
	return copy
}
