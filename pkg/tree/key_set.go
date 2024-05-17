package tree

import "strings"

type PathSet struct {
	index map[string]struct{}
	paths [][]string
}

func NewPathSet() *PathSet {
	return &PathSet{
		index: map[string]struct{}{},
		paths: [][]string{},
	}
}

func (p *PathSet) AddPath(path []string) {
	k := strings.Join(path, KeysIndexSep)
	if _, exists := p.index[k]; !exists {
		p.paths = append(p.paths, path)
		p.index[k] = struct{}{}
	}
}

func (p *PathSet) Join(other *PathSet) {
	for _, x := range other.paths {
		p.AddPath(x)
	}
}

func (p *PathSet) GetPaths() [][]string {
	return p.paths
}
