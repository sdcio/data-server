package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (s *sharedEntryAttributes) BreadthSearch(ctx context.Context, path string) ([]Entry, error) {
	var err error
	var resultEntries []Entry
	var processEntries []Entry

	lref, err := utils.StripPathElemPrefix(path)
	if err != nil {
		return nil, fmt.Errorf("failed stripping namespaces from leafref %s: %w", s.Path(), err)
	}

	sdcpbPath, err := utils.ParsePath(lref)
	if err != nil {
		return nil, fmt.Errorf("failed parsing leafref path %s: %w", s.Path(), err)
	}

	lrefPath := types.NewLrefPath(sdcpbPath)

	// if the lrefs first character is "/" then it is a root based path
	isRootBasedPath := false
	if string(lref[0]) == "/" {
		isRootBasedPath = true
	}

	if isRootBasedPath {
		processEntries = []Entry{s.GetRoot()}
	} else {
		var entry Entry = s
		dotdotcount := 0
		sdcpbUp := []*sdcpb.PathElem{}
		// process the .. instructions
		for _, elem := range lrefPath {
			// if we've processed all the .., we're done here
			// break and continue
			if elem.Name != ".." {
				break
			}
			sdcpbUp = append(sdcpbUp, &sdcpb.PathElem{Name: ".."})
			dotdotcount++
		}
		// else navigate (basically up)
		entry, err = entry.NavigateSdcpbPath(ctx, sdcpbUp, false)
		if err != nil {
			return nil, err
		}
		processEntries = []Entry{entry}
		lrefPath = lrefPath[dotdotcount:]
	}

	count := 0
	// forward the path by a single element
	for _, elem := range lrefPath {

		resultEntries = []Entry{}
		err := s.resolve_leafref_key_path(ctx, elem.Keys)
		if err != nil {
			return nil, err
		}

		// we need to do the forwarding for all the already lookedup paths
		for _, entry := range processEntries {
			entry, err = entry.Navigate(ctx, []string{elem.Name}, false, false)
			if err != nil {
				return nil, err
			}

			// if the entry is will be deleted by the actual operation, skip it.
			if !entry.remainsToExist() {
				continue
			}
			// if we're at the final level, no child filtering is needed any more,
			// just return the result
			if count == len(lrefPath)-1 {
				resultEntries = append(resultEntries, entry)
				continue
			}
			var childs []Entry
			// if the entry is a list with keys, try filtering the entries based on the keys
			if len(entry.GetSchemaKeys()) > 0 {
				// filter the keys
				childs, err = entry.FilterChilds(elem.KeysToMap())
				if err != nil {
					return nil, err
				}
			} else {
				// if no keys are present,
				// add the entry to the childs list
				childs = append(childs, entry)
			}
			resultEntries = append(resultEntries, childs...)
		}
		processEntries = resultEntries
		count++
	}
	return resultEntries, nil
}

// NavigateLeafRef
func (s *sharedEntryAttributes) NavigateLeafRef(ctx context.Context) ([]Entry, error) {

	// leafref path takes as an argument a string that MUST refer to a leaf or leaf-list node.
	// e.g.
	// leaf foo {
	//   type leafref {
	//     path "/bar/baz";
	//   }
	// }
	// the runtime semantics are:
	// If /bar/baz resolves to a leaf, then foo must have exactly the same value as that leaf.
	// If /bar/baz resolves to a leaf-list, then foo must have a value equal to one of the existing entries in that leaf-list.
	// Thus, the "pointer" is not to the entire leaf-list node, but to one instance of it, selected by value.

	var lref string
	switch {
	case s.GetSchema().GetField().GetType().GetLeafref() != "":
		lref = s.schema.GetField().GetType().GetLeafref()
	case s.GetSchema().GetLeaflist().GetType().GetLeafref() != "":
		lref = s.GetSchema().GetLeaflist().GetType().GetLeafref()
	default:
		return nil, fmt.Errorf("error not a leafref %s", s.Path().String())
	}

	lv := s.leafVariants.GetHighestPrecedence(false, true, false)
	if lv == nil {
		return nil, fmt.Errorf("no leafvariant found")
	}
	// value of node with type leafref
	tv := lv.Value()

	foundEntries, err := s.BreadthSearch(ctx, lref)
	if err != nil {
		return nil, err
	}

	var resultEntries []Entry

	for _, e := range foundEntries {
		r, err := e.getHighestPrecedenceLeafValue(ctx)
		if err != nil {
			return nil, err
		}
		// Value of the resolved leafref
		refVal := r.Value()
		var vals []*sdcpb.TypedValue

		switch tVal := refVal.Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			vals = append(vals, tVal.LeaflistVal.GetElement()...)
		default:
			vals = append(vals, refVal)
		}
		// loop through possible values of found reference (leaf -> 1 value, leaf-list -> 1+ values)
		for _, val := range vals {
			if utils.EqualTypedValues(val, tv) {
				resultEntries = append(resultEntries, e)
				break
			}
		}
	}

	return resultEntries, nil
}

func (s *sharedEntryAttributes) resolve_leafref_key_path(ctx context.Context, keys map[string]*types.LrefPathElemKeyValue) error {
	// resolve keys
	for k, v := range keys {
		if v.DoNotResolve || !strings.Contains(v.Value, "/") || !strings.Contains(v.Value, "current") {
			continue
		}
		isRootPath := true

		var keyp *sdcpb.Path
		keyp, err := utils.ParsePath(v.Value)
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

		lvs := keyValue.GetHighestPrecedence(LeafVariantSlice{}, false, false, false)
		if lvs == nil {
			return fmt.Errorf("no leafentry found")
		}
		tv := lvs[0].Value()
		keys[k].Value = tv.GetStringVal()
		keys[k].DoNotResolve = true
	}
	return nil
}

func (s *sharedEntryAttributes) validateLeafRefs(ctx context.Context, resultChan chan<- *types.ValidationResultEntry, statChan chan<- *types.ValidationStat) {
	if s.shouldDelete() {
		return
	}

	lref := s.schema.GetField().GetType().GetLeafref()
	if s.schema == nil || lref == "" {
		return
	}
	statChan <- types.NewValidationStat(types.StatTypeLeafRef).PlusOne()
	entry, err := s.NavigateLeafRef(ctx)
	if err != nil || len(entry) == 0 {
		// check if the OptionalInstance (!require-instances [https://datatracker.ietf.org/doc/html/rfc7950#section-9.9.3])
		if s.schema.GetField().GetType().GetOptionalInstance() {
			generateOptionalWarning(ctx, s, lref, resultChan)
			return
		}
		owner := "unknown"
		highest := s.leafVariants.GetHighestPrecedence(false, false, false)
		if highest != nil {
			owner = highest.Owner()
		}
		// if required, issue error
		resultChan <- types.NewValidationResultEntry(owner, fmt.Errorf("missing leaf reference: failed resolving leafref %s for %s: %v", lref, s.Path().String(), err), types.ValidationResultEntryTypeError)
		return
	}

	// Only if the value remains, even after the SetIntent made it through, the LeafRef can be considered resolved.
	if entry[0].shouldDelete() {
		lv := s.leafVariants.GetHighestPrecedence(false, true, false)
		if lv == nil {
			return
		}
		EntryPath, _ := s.SdcpbPath()

		// check if the OptionalInstance (!require-instances [https://datatracker.ietf.org/doc/html/rfc7950#section-9.9.3])
		if s.schema.GetField().GetType().GetOptionalInstance() {
			generateOptionalWarning(ctx, s, lref, resultChan)
			return
		}
		// if required, issue error
		resultChan <- types.NewValidationResultEntry(lv.Owner(), fmt.Errorf("missing leaf reference: failed resolving leafref %s for %s to path %s LeafVariant %v", lref, utils.ToXPath(EntryPath, false), s.Path().String(), lv), types.ValidationResultEntryTypeError)
		return
	}
}

func generateOptionalWarning(ctx context.Context, s Entry, lref string, resultChan chan<- *types.ValidationResultEntry) {
	lrefval, err := s.getHighestPrecedenceLeafValue(ctx)
	if err != nil {
		resultChan <- types.NewValidationResultEntry(lrefval.Owner(), err, types.ValidationResultEntryTypeError)
		return
	}
	tvVal := lrefval.Value()
	resultChan <- types.NewValidationResultEntry(lrefval.Owner(), fmt.Errorf("leafref %s value %s unable to resolve non-mandatory reference %s", s.Path().String(), utils.TypedValueToString(tvVal), lref), types.ValidationResultEntryTypeWarning)
}
