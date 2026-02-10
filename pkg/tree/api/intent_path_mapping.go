package api

import (
	"iter"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeletePathSet struct {
	data map[string]*DeletePathPrio
}

func NewDeletePaths() *DeletePathSet {
	return &DeletePathSet{
		data: map[string]*DeletePathPrio{},
	}
}

func (dp *DeletePathSet) DeepCopy() *DeletePathSet {
	result := NewDeletePaths()
	result.data = map[string]*DeletePathPrio{}
	for k, v := range dp.data {
		result.data[k] = v.DeepCopy()
	}
	return result
}

func (dp *DeletePathSet) RemoveIntentDeletes(intentName string) *sdcpb.PathSet {
	if data, exists := dp.data[intentName]; exists {
		result := data.GetPathSet()
		delete(dp.data, intentName)
		return result
	}
	return sdcpb.NewPathSet()
}

func (dp *DeletePathSet) Add(intentName string, prio int32, pathset *sdcpb.PathSet) {
	dpp, exists := dp.data[intentName]
	if !exists {
		dpp = NewDeletePathPrio(intentName, prio)
		dp.data[intentName] = dpp
	}
	if pathset == nil {
		return
	}
	dpp.paths.Join(pathset)
}

func (dp *DeletePathSet) GetByIntentName(intentName string) *sdcpb.PathSet {
	data, exists := dp.data[intentName]
	if exists {
		return data.paths
	}
	return sdcpb.NewPathSet()
}

func (dp *DeletePathSet) Items() iter.Seq[*DeletePathPrio] {
	return func(yield func(*DeletePathPrio) bool) {
		for _, val := range dp.data {
			if !yield(val) {
				return
			}
		}
	}
}

type DeletePathPrio struct {
	owner string
	prio  int32
	paths *sdcpb.PathSet
}

func NewDeletePathPrio(owner string, prio int32) *DeletePathPrio {
	return &DeletePathPrio{
		prio:  prio,
		owner: owner,
		paths: sdcpb.NewPathSet(),
	}
}

func (ddp *DeletePathPrio) DeepCopy() *DeletePathPrio {
	result := NewDeletePathPrio(ddp.GetOwner(), ddp.GetPrio())
	result.paths = ddp.paths.DeepCopy()
	return result
}

func (dpp *DeletePathPrio) GetPathSet() *sdcpb.PathSet {
	return dpp.paths
}

func (dpp *DeletePathPrio) PathItems() iter.Seq[*sdcpb.Path] {
	return dpp.paths.Items()
}

func (dpp *DeletePathPrio) GetPrio() int32 {
	return dpp.prio
}

func (dpp *DeletePathPrio) GetOwner() string {
	return dpp.owner
}
