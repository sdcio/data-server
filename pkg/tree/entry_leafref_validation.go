package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *sharedEntryAttributes) validateLeafRefs(ctx context.Context, errchan chan<- error) {
	var err error

	lref := s.schema.GetField().GetType().GetLeafref()
	if s.schema == nil || s.schema.GetField().GetType().GetLeafref() == "" {
		return
	}

	lv := s.leafVariants.GetHighestPrecedence(false)

	lref, err = utils.StripPathElemPrefix(lref)
	if err != nil {
		errchan <- fmt.Errorf("failed stripping namespaces from leafref %s LeafVariant %v: %w", s.Path(), lv, err)
		return
	}

	lrefSdcpbPath, err := utils.ParsePath(lref)
	if err != nil {
		errchan <- fmt.Errorf("failed parsing leafref path %s LeafVariant %v: %w", s.Path(), lv, err)
		return
	}

	// if the lrefs first character is "/" then it is a root based path
	isRootBasedPath := false
	if string(lref[0]) == "/" {
		isRootBasedPath = true
	}

	strPath := utils.ToStrings(lrefSdcpbPath, false, false)

	tv, err := lv.Update.Value()
	if err != nil {
		errchan <- fmt.Errorf("failed reading value from %s LeafVariant %v: %w", s.Path(), lv, err)
		return
	}
	value := tv.GetStringVal()

	if !isRootBasedPath {
		basePath := s.Path()
		basePath = basePath[:len(basePath)-1]
		for _, p := range strPath {
			if p == ".." {
				basePath = basePath[:len(basePath)-1]
				continue
			}
			basePath = append(basePath, p)
		}
		strPath = basePath
		isRootBasedPath = true
	}

	strPath = append(strPath[:len(strPath)-1], value, strPath[len(strPath)-1])

	sdcpbP, err := s.treeContext.treeSchemaCacheClient.ToPath(ctx, strPath)
	if err != nil {
		errchan <- fmt.Errorf("failed converting ToPath %s LeafVariant %v: %w", s.Path(), lv, err)
		return
	}

	lrefPath := newLrefPath(sdcpbP)
	lrefPath[len(lrefPath)-2].Keys[lrefPath[len(lrefPath)-1].Name] = &lrefPathElemKeyValue{doNotResolve: true, value: value}

	err = s.resolve_leafref_key_path(ctx, &lrefPath)
	if err != nil {
		errchan <- fmt.Errorf("failed resolving leafref keys %s LeafVariant %v: %w", s.Path(), lv, err)
		return
	}

	// convert back to sdcpb.Path
	sdcpbP.Elem = lrefPath.ToSdcpbPathElem()

	// navigate to the given path, check if it exists by evaluating the error
	entry, err := s.NavigateSdcpbPath(ctx, sdcpbP.Elem, isRootBasedPath)
	if err != nil {
		EntryPath, _ := s.SdcpbPath()
		errchan <- fmt.Errorf("missing leaf reference: failed resolving leafref for %s to path %s LeafVariant %v: %w", utils.ToXPath(EntryPath, false), utils.ToXPath(sdcpbP, false), lv, err)
		return
	}

	// Only if the value remains, even after the SetIntent made it through, the LeafRef can be considered resolved.
	if !entry.remainsToExist() {
		EntryPath, _ := s.SdcpbPath()
		errchan <- fmt.Errorf("missing leaf reference: failed resolving leafref for %s to path %s LeafVariant %v", utils.ToXPath(EntryPath, false), utils.ToXPath(sdcpbP, false), lv)
		return
	}

}

// lrefPath for the leafref resolution we need to distinguish between already resolved values and not yet resolved xpath statements
// this is a struct that provides this information
type lrefPath []*lrefPathElem

type lrefPathElem struct {
	Name string
	Keys map[string]*lrefPathElemKeyValue
}

type lrefPathElemKeyValue struct {
	doNotResolve bool
	value        string
}

func newLrefPath(p *sdcpb.Path) lrefPath {
	lp := lrefPath{}
	for _, x := range p.Elem {
		lrefpe := &lrefPathElem{
			Name: x.Name,
			Keys: map[string]*lrefPathElemKeyValue{},
		}
		for k, v := range x.Key {
			lrefpe.Keys[k] = &lrefPathElemKeyValue{
				value:        v,
				doNotResolve: false,
			}
		}
		lp = append(lp, lrefpe)
	}

	return lp

}

func (pes lrefPath) ToSdcpbPathElem() []*sdcpb.PathElem {
	result := make([]*sdcpb.PathElem, 0, len(pes))
	for _, e := range pes {
		pe := &sdcpb.PathElem{
			Name: e.Name,
			Key:  map[string]string{},
		}
		for k, v := range e.Keys {
			pe.Key[k] = v.value
		}
		result = append(result, pe)
	}
	return result
}

func (s *sharedEntryAttributes) resolve_leafref_key_path(ctx context.Context, p *lrefPath) error {

	// process leafref elements and adjust result
	for _, lrefElem := range *p {

		// resolve keys
		for k, v := range lrefElem.Keys {

			if v.doNotResolve || !strings.Contains(v.value, "/") || !strings.Contains(v.value, "current") {
				continue
			}
			isRootPath := true

			var keyp *sdcpb.Path
			keyp, err := utils.ParsePath(v.value)
			if err != nil {
				return err
			}

			// replace current() with its actual value
			if strings.ToLower(keyp.Elem[0].Name) == "current()" {
				keyp.Elem = keyp.Elem[1:]
				isRootPath = false
			}

			keyValue, err := s.NavigateSdcpbPath(ctx, keyp.Elem, isRootPath)
			if err != nil {
				return err
			}

			lvs := keyValue.GetHighestPrecedence(LeafVariantSlice{}, false)
			tv, err := lvs[0].Value()
			if err != nil {
				return err
			}
			lrefElem.Keys[k].value = tv.GetStringVal()
			lrefElem.Keys[k].doNotResolve = true
		}
	}
	return nil
}
