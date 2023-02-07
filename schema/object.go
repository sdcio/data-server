package schema

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/goyang/pkg/yang"
	log "github.com/sirupsen/logrus"
)

func ObjectFromYEntry(e *yang.Entry) any {
	switch {
	case e.IsLeaf():
		return leafFromYEntry(e)
	case e.IsLeafList():
		return leafListFromYEntry(e)
	default:
		return containerFromYEntry(e)
	}
}

func (sc *Schema) GetEntry(pe []string) (*yang.Entry, error) {
	if len(pe) == 0 {
		return sc.root, nil
	}
	first := pe[0]
	offset := 1
	index := strings.Index(pe[0], ":")
	if index > 0 {
		first = pe[0][:index]
		pe[0] = pe[0][index+1:]
		offset = 0
	}

	sc.m.RLock()
	defer sc.m.RUnlock()
	if e, ok := sc.root.Dir[first]; ok {
		if e == nil {
			return nil, fmt.Errorf("module %q not found", first)
		}
		return getEntry(e, pe[offset:])
	}
	// skip first level modules and try their children
	for _, child := range sc.root.Dir {
		if cc, ok := child.Dir[first]; ok {
			return getEntry(cc, pe[offset:])
		}
	}
	return nil, fmt.Errorf("entry %q not found", pe[0])
}

func getEntry(e *yang.Entry, pe []string) (*yang.Entry, error) {
	log.Debugf("getEntry %s Dir=%v, Choice=%v, Case=%v, %v",
		e.Name,
		e.IsDir(),
		e.IsChoice(),
		e.IsCase(),
		pe)
	switch len(pe) {
	case 0:
		switch {
		case e.IsCase(), e.IsChoice():
			if ee := e.Dir[e.Name]; ee != nil {
				return ee, nil
			}
		case e.IsContainer():
			if ee := e.Dir[e.Name]; ee != nil {
				if ee.IsCase() || ee.IsChoice() {
					return ee, nil
				}
			}
		}
		return e, nil
	default:
		if e.Dir == nil {
			return nil, errors.New("not found")
		}
		for _, ee := range getChildren(e) {
			// fmt.Printf("entry %s, child %s | %s\n", e.Name, ee.Name, pe)
			if ee.Name != pe[0] {
				continue
			}
			return getEntry(ee, pe[1:])
		}
		// fmt.Println("entry name", e.Name, pe)
		return nil, fmt.Errorf("%q not found", pe[0])
	}
}

func (sc *Schema) BuildPath(pe []string, p *schemapb.Path) error {
	if len(pe) == 0 {
		return nil
	}
	sc.m.RLock()
	defer sc.m.RUnlock()
	if p.GetElem() == nil {
		p.Elem = make([]*schemapb.PathElem, 0, 1)
	}
	for _, e := range sc.root.Dir {
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe, p, ee)
		}
	}

	return nil
}

func (sc *Schema) buildPath(pe []string, p *schemapb.Path, e *yang.Entry) error {
	lpe := len(pe)
	cpe := &schemapb.PathElem{
		Name: e.Name,
	}
	if lpe == 0 {
		p.Elem = append(p.Elem, cpe)
		return nil
	}
	switch {
	case e.IsList():
		if cpe.GetKey() == nil {
			cpe.Key = make(map[string]string)
		}
		p.Elem = append(p.Elem, cpe)
		keys := strings.Fields(e.Key)
		sort.Strings(keys)
		count := 1
		for i, k := range keys {
			if i+1 >= lpe {
				break
			}
			count++
			cpe.Key[k] = pe[i+1]
		}
		if lpe == count {
			return nil
		}
		nxt := pe[count]
		if ee, ok := e.Dir[nxt]; ok {
			return sc.buildPath(pe[count:], p, ee)
		}

	case e.IsCase():
		// p.Elem = append(p.Elem, cpe)
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe, p, ee)
		}
	case e.IsChoice():
		p.Elem = append(p.Elem, cpe)
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe[1:], p, ee)
		}
	case e.IsContainer():
		if ee, ok := e.Dir[e.Name]; ee != nil && ok {
			if ee.IsCase() || ee.IsChoice() {
				return sc.buildPath(pe[1:], p, ee)
			}
		}
		p.Elem = append(p.Elem, cpe)
		if lpe == 1 {
			return nil
		}
		if ee, ok := e.Dir[pe[1]]; ok {
			return sc.buildPath(pe[1:], p, ee)
		} else {
			for _, ee := range e.Dir {
				if ee.IsCase() || ee.IsChoice() {
					if eee, ok := ee.Dir[pe[1]]; ok {
						return sc.buildPath(pe[1:], p, eee)
					}
				}
			}
		}
		return fmt.Errorf("unknown element %v", pe[1])
	case e.IsLeaf():
		p.Elem = append(p.Elem, cpe)
		if lpe != 1 {
			return fmt.Errorf("unknown element %v", pe[0])
		}
	case e.IsLeafList():
		p.Elem = append(p.Elem, cpe)
		if lpe != 1 {
			return fmt.Errorf("unknown element %v", pe[0])
		}
	}
	return nil
}

func getChildren(e *yang.Entry) []*yang.Entry {
	switch {
	case e.IsChoice():
		rs := make([]*yang.Entry, 0, len(e.Dir))
		for _, ee := range e.Dir {
			if ee.IsChoice() || ee.IsCase() {
				rs = append(rs, getChildren(ee)...)
				continue
			}
			rs = append(rs, ee)
		}
		//sort.Slice(rs, sortFn(rs))
		return rs
	case e.IsCase():
		rs := make([]*yang.Entry, 0, len(e.Dir))
		for _, ee := range e.Dir {
			if ee.IsChoice() || ee.IsCase() {
				rs = append(rs, getChildren(ee)...)
				continue
			}
			rs = append(rs, ee)
		}
		//sort.Slice(rs, sortFn(rs))
		return rs
	case e.IsContainer():
		rs := make([]*yang.Entry, 0, len(e.Dir))
		for _, ee := range e.Dir {
			if ee.IsChoice() || ee.IsCase() {
				rs = append(rs, getChildren(ee)...)
				continue
			}
			rs = append(rs, ee)
		}
		//sort.Slice(rs, sortFn(rs))
		return rs
	case e.IsList():
		rs := make([]*yang.Entry, 0, len(e.Dir))
		for _, ee := range e.Dir {
			// fmt.Printf("!! Appending child %s to %s\n", ee.Name, e.Name)
			if ee.IsChoice() || ee.IsCase() {
				rs = append(rs, getChildren(ee)...)
				continue
			}
			// fmt.Printf("Appending child %s to %s\n", ee.Name, e.Name)
			rs = append(rs, ee)
		}
		//sort.Slice(rs, sortFn(rs))
		return rs
	default:
		return nil
	}
}

func getParent(e *yang.Entry) *yang.Entry {
	if e == nil {
		return nil
	}
	if e.Parent != nil && e.Parent.Name == "root" {
		return nil
	}
	// if !e.IsChoice() && !e.IsCase() {
	// 	return e.Parent
	// }
	// if e.Parent.IsCase() || e.Parent.IsChoice() {
	// 	fmt.Println("getParent Parent", e.Parent.Name, e.Parent.IsChoice(), e.Parent.IsCase())
	// 	return getParent(e.Parent)
	// }
	return e.Parent
}

func sortFn(rs []*yang.Entry) func(i, j int) bool {
	return func(i, j int) bool {
		switch {
		case rs[i].IsLeaf():
			switch {
			case rs[j].IsLeaf():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsChoice():
			switch {
			case rs[j].IsLeaf():
				return false
			case rs[j].IsChoice():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsCase():
			switch {
			case rs[j].IsLeaf():
				return false
			case rs[j].IsChoice():
				return false
			// case rs[j].IsLeafList():
			// 	return false
			case rs[j].IsCase():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsContainer():
			switch {
			case rs[j].IsContainer():
				return rs[i].Name < rs[j].Name
			default:
				return false
			}
		default:
			return false
		}
	}
}
