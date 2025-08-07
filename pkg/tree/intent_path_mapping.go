package tree

import (
	"iter"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

type DeletePaths struct {
	data map[string]*sdcpb.PathSet
}

func NewDeletePaths() *DeletePaths {
	return &DeletePaths{
		data: map[string]*sdcpb.PathSet{},
	}
}

func (dp *DeletePaths) Add(intentName string, pathset *sdcpb.PathSet) {
	set, exists := dp.data[intentName]
	if !exists {
		set = sdcpb.NewPathSet()
		dp.data[intentName] = set
	}
	set.Join(pathset)
}

func (dp *DeletePaths) Items() iter.Seq[*sdcpb.Path] {
	return func(yield func(*sdcpb.Path) bool) {
		for _, val := range dp.data {
			for path := range val.Items() {
				if !yield(path) {
					return
				}
			}
		}
	}
}
