package schema

import (
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/openconfig/goyang/pkg/yang"
	log "github.com/sirupsen/logrus"
)

func (sc *Schema) ExpandPath(p *schemapb.Path, dt schemapb.DataType) ([]*schemapb.Path, error) {
	ps := make([]*schemapb.Path, 0)
	cp := utils.ToStrings(p, false, true)
	e, err := sc.GetEntry(cp)
	if err != nil {
		return nil, err
	}
	populatePathKeys(e, p)
	switch {
	case e.IsLeaf():
		return []*schemapb.Path{p}, nil
	}
	for _, c := range e.Dir {
		for _, pe := range sc.getPathElems(c, dt) {
			np := &schemapb.Path{
				Elem: make([]*schemapb.PathElem, 0, len(p.GetElem())+len(pe)),
			}
			np.Elem = append(np.Elem, p.GetElem()...)
			np.Elem = append(np.Elem, pe...)
			ps = append(ps, np)
		}
	}
	return ps, nil
}

func (sc *Schema) getPathElems(e *yang.Entry, dt schemapb.DataType) [][]*schemapb.PathElem {
	rs := make([][]*schemapb.PathElem, 0)
	switch {
	case e.IsCase():
		log.Debugf("got case: %s", e.Name)
		for _, c := range e.Dir {
			rs = append(rs, sc.getPathElems(c, dt)...)
		}
	case e.IsChoice():
		log.Debugf("got choice: %s", e.Name)
		for _, c := range e.Dir {
			rs = append(rs, sc.getPathElems(c, dt)...)
		}
	case e.IsLeaf():
		log.Debugf("got leaf: %s", e.Name)
		switch dt {
		case schemapb.DataType_ALL:
		case schemapb.DataType_CONFIG:
			if isState(e) {
				return nil
			}
		case schemapb.DataType_STATE:
			if !isState(e) {
				return nil
			}
		}
		return [][]*schemapb.PathElem{{&schemapb.PathElem{Name: e.Name}}}
	case e.IsLeafList():
		log.Debugf("got leafList: %s", e.Name)
		switch dt {
		case schemapb.DataType_ALL:
		case schemapb.DataType_CONFIG:
			if isState(e) {
				return nil
			}
		case schemapb.DataType_STATE:
			if !isState(e) {
				return nil
			}
		}
		return [][]*schemapb.PathElem{{&schemapb.PathElem{Name: e.Name}}}
	case e.IsList():
		log.Debugf("got list: %s", e.Name)
		listPE := &schemapb.PathElem{Name: e.Name, Key: make(map[string]string)}
		keys := strings.Fields(e.Key)
		kmap := make(map[string]struct{})
		for _, k := range keys {
			listPE.Key[k] = "*"
			kmap[k] = struct{}{}
		}

		for _, c := range e.Dir {
			if _, ok := kmap[c.Name]; ok {
				continue
			}
			log.Debugf("list parent adding child: %s", c.Name)

			childrenPE := sc.getPathElems(c, dt)
			for _, cpe := range childrenPE {
				branch := make([]*schemapb.PathElem, 0, len(cpe)+1)
				branch = append(branch, listPE)
				branch = append(branch, cpe...)
				rs = append(rs, branch)
			}
		}
	case e.IsContainer():
		log.Debugf("got container: %s", e.Name)
		containerPE := &schemapb.PathElem{Name: e.Name, Key: make(map[string]string)}
		for _, c := range e.Dir {
			log.Debugf("container parent adding child: %s", c.Name)
			childrenPE := sc.getPathElems(c, dt)

			for _, cpe := range childrenPE {
				branch := make([]*schemapb.PathElem, 0, len(cpe)+1)
				branch = append(branch, containerPE)
				branch = append(branch, cpe...)
				rs = append(rs, branch)
			}
		}
	}
	return rs
}

func populatePathKeys(e *yang.Entry, p *schemapb.Path) {
	ce := e
	for i := len(p.GetElem()) - 1; i >= 0; i-- {
		if ce.Parent != nil && ce.Parent.Name == "root" {
			return
		}
		populatePathElemKeys(ce, p.GetElem()[i])
		ce = ce.Parent
	}
}

func populatePathElemKeys(e *yang.Entry, pe *schemapb.PathElem) {
	switch {
	case e.IsList():
		for _, k := range strings.Fields(e.Key) {
			if pe.GetKey() == nil {
				pe.Key = make(map[string]string)
			}
			if _, ok := pe.GetKey()[k]; !ok {
				pe.Key[k] = "*"
			}
		}
	}
}
