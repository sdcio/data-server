package schema

import (
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/openconfig/goyang/pkg/yang"
	log "github.com/sirupsen/logrus"
)

func (sc *Schema) buildReferencesAnnotation() error {
	var err error
	sc.m.RLock()
	defer sc.m.RUnlock()
	for _, e := range sc.root.Dir {
		err = sc.buildReferences(e)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sc *Schema) buildReferences(e *yang.Entry) error {
	if isState(e) {
		return nil
	}
	// fmt.Println("buildref: ", e.Name)
	if e.Type != nil && yang.TypeKind(e.Type.Kind).String() == "leafref" {
		pes := normalizePath(e.Type.Path, e)
		// fmt.Println("original path  :", e.Type.Path)
		// fmt.Println("normalized path:", pes)
		refEntry, err := sc.GetEntry(pes)
		if err != nil {
			return err
		}
		// fmt.Println("got refEntry", refEntry.Name)
		if refEntry.Annotation == nil {
			refEntry.Annotation = make(map[string]interface{})
		}
		refEntry.Annotation["REF_"+e.Path()] = e
	}
	for _, ce := range e.Dir {
		if ce.IsCase() || ce.IsChoice() {
			for _, cce := range ce.Dir {
				err := sc.buildReferences(cce)
				if err != nil {
					return err
				}
			}
		}
		err := sc.buildReferences(ce)
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizePath(p string, e *yang.Entry) []string {
	scp, _ := utils.ParsePath(p)
	if hasRelativePathElem(scp) {
		scp = relativeToAbsPath(scp, e)
	}
	if hasRelativeKeys(scp) {
		relativeToAbsPathKeys(scp, e)
	}
	for _, pe := range scp.GetElem() {
		if spe := strings.SplitN(pe.Name, ":", 2); len(spe) == 2 {
			pe.Name = spe[1]
		}
	}
	pe := utils.ToStrings(scp, false, true)
	return pe
}

func relativeToAbsPathKeys(p *schemapb.Path, e *yang.Entry) {
	// go through the Path elements
	for _, pe := range p.GetElem() {

		// check all keys in the path element
		for k, v := range pe.GetKey() {
			// if the actual path element does not contain a relative ref
			// continue with next pe otherwise go on
			if !strings.Contains(v, "current()") && !strings.Contains(v, "..") {
				continue
			}

			// split path into its elements
			keyPath, err := utils.ParsePath(strings.TrimSpace(v))
			if err != nil {
				log.Error(err)
			}

			// current yang entry will be forwarded via key path elements
			ce := e

			// iterate over the Key referenced path elements
			// forward the cye accordingly.
			// cye will finally contain the yang entry that the key referres to
			for _, kpe := range keyPath.Elem {
				switch kpe.Name {
				case "current()":
					for (ce.IsCase() || ce.IsChoice()) && ce.Parent != nil {
						ce = ce.Parent
					}
				case "..":
					for (ce.IsCase() || ce.IsChoice()) && ce.Parent != nil {
						ce = ce.Parent
					}
					ce = ce.Parent
				default:
					// remove module name from PathElement
					kpeParts := strings.SplitN(kpe.Name, ":", 2)
					// default to [0]
					kpeName := kpeParts[0]
					// if module name present len will be 2
					// MODULENAME:ELEMENTNAME -> so [1] is to be used
					if len(kpeParts) == 2 {
						kpeName = kpeParts[1]
					}
					// forward the current yang entry "pointer"
					ce = ce.Dir[kpeName]
				}
			}

			// replace the PathElements Key with the Absolute Path to the Key Value
			pe.Key[k] = ce.Path()
		}
	}
}

func relativeToAbsPath(p *schemapb.Path, e *yang.Entry) *schemapb.Path {
	np := &schemapb.Path{
		Elem: make([]*schemapb.PathElem, 0, len(p.GetElem())),
	}
	ce := e
	for _, pe := range p.GetElem() {
		if ce == nil {
			break
		}
		if pe.Name == ".." {
			// fmt.Println("relative path @E", ce.Name, e.IsCase(), e.IsChoice())
			for (ce.IsCase() || ce.IsChoice()) && ce.Parent != nil {
				ce = ce.Parent.Parent
			}
			ce = ce.Parent
			continue
		}
		// fmt.Println("ce | np", ce.Name, np)
		np.Elem = append(np.Elem, pe)
	}
	if ce == nil {
		return np
	}
	for ce != nil && ce.Parent != nil && ce.Parent.Name != "root" {
		np.Elem = append([]*schemapb.PathElem{{Name: ce.Name}}, np.GetElem()...)
		ce = ce.Parent
	}
	return np
}

func hasRelativePathElem(p *schemapb.Path) bool {
	for _, pe := range p.GetElem() {
		if pe.GetName() == ".." {
			return true
		}
	}
	return false
}

func hasRelativeKeys(p *schemapb.Path) bool {
	for _, pe := range p.GetElem() {
		for _, v := range pe.GetKey() {
			if strings.Contains(v, "..") || strings.Contains(v, "current()") {
				return true
			}
		}
	}
	return false
}

func buildPathUpFromEntry(e *yang.Entry) *schemapb.Path {
	if e == nil {
		return nil
	}
	p := &schemapb.Path{Elem: make([]*schemapb.PathElem, 0, 16)}
	p.Elem = append(p.Elem, &schemapb.PathElem{Name: e.Name})
	pce := e
	for pce != nil {
		pce = getParent(pce)
		if pce == nil {
			break
		}
		if pce.IsCase() || pce.IsChoice() {
			continue
		}
		p.Elem = append(p.Elem, &schemapb.PathElem{Name: pce.Name})
	}
	// reverse slice
	for i, j := 0, len(p.GetElem())-1; i < j; i, j = i+1, j-1 {
		p.Elem[i], p.Elem[j] = p.Elem[j], p.Elem[i]
	}
	return p
}
