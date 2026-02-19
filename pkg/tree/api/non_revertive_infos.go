package api

import "github.com/sdcio/sdc-protos/sdcpb"

type NonRevertiveInfos map[string]*NonRevertiveInfo

func NewNonRevertiveInfos() NonRevertiveInfos {
	return make(map[string]*NonRevertiveInfo)
}

func (n NonRevertiveInfos) Add(owner string, nonRevertive bool) {
	_, ok := n[owner]
	if !ok {
		n[owner] = NewNonRevertiveInfo(owner, nonRevertive)
	}
}

func (n NonRevertiveInfos) AddNonRevertivePath(owner string, path *sdcpb.Path) {
	info, ok := n[owner]
	if !ok {
		info = NewNonRevertiveInfo(owner, false)
		n[owner] = info
	}
	info.AddPath(path)
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
