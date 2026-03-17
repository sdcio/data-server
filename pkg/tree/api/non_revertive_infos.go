package api

import (
	"strings"

	"github.com/sdcio/sdc-protos/sdcpb"
)

type NonRevertiveInfos map[string]*NonRevertiveInfo

func NewNonRevertiveInfos() NonRevertiveInfos {
	return make(map[string]*NonRevertiveInfo)
}

func (n NonRevertiveInfos) String() string {
	var sb strings.Builder
	for _, info := range n {
		sb.WriteString(info.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

func (n NonRevertiveInfos) Add(owner string, nonRevertive bool, paths ...*sdcpb.Path) {
	_, ok := n[owner]
	if !ok {
		n[owner] = NewNonRevertiveInfo(owner, nonRevertive)
	} else {
		n[owner].SetNonRevertive(nonRevertive)
	}
	n[owner].AddPaths(paths...)
}

func (n NonRevertiveInfos) IsNonRevertive(owner string, path SdcpbPath) bool {
	info, ok := n[owner]
	if !ok {
		return false
	}
	return info.IsNonRevertive(path)
}

func (n NonRevertiveInfos) IsGenerallyNonRevertive(owner string) bool {
	info, ok := n[owner]
	if !ok {
		return false
	}
	return info.IsGenerallyNonRevertive()
}

func (n NonRevertiveInfos) DeepCopy() NonRevertiveInfos {
	m := make(NonRevertiveInfos, len(n))
	for k, v := range n {
		m[k] = v.DeepCopy()
	}
	return m
}
