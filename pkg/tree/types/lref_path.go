package types

import (
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// LrefPath for the leafref resolution we need to distinguish between already resolved values and not yet resolved xpath statements
// this is a struct that provides this information
type LrefPath []*LrefPathElem

type LrefPathElem struct {
	Name string
	Keys map[string]*LrefPathElemKeyValue
}

func (l *LrefPathElem) KeysToMap() map[string]string {
	result := map[string]string{}
	for k, v := range l.Keys {
		result[k] = v.Value
	}
	return result
}

type LrefPathElemKeyValue struct {
	DoNotResolve bool
	Value        string
}

func NewLrefPath(p *sdcpb.Path) LrefPath {
	lp := LrefPath{}
	for _, x := range p.Elem {
		lrefpe := &LrefPathElem{
			Name: x.Name,
			Keys: map[string]*LrefPathElemKeyValue{},
		}
		for k, v := range x.Key {
			lrefpe.Keys[k] = &LrefPathElemKeyValue{
				Value:        v,
				DoNotResolve: false,
			}
		}
		lp = append(lp, lrefpe)
	}

	return lp

}

func (pes LrefPath) ToSdcpbPathElem() []*sdcpb.PathElem {
	result := make([]*sdcpb.PathElem, 0, len(pes))
	for _, e := range pes {
		pe := &sdcpb.PathElem{
			Name: e.Name,
			Key:  map[string]string{},
		}
		for k, v := range e.Keys {
			pe.Key[k] = v.Value
		}
		result = append(result, pe)
	}
	return result
}
