package ops

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sdcio/data-server/pkg/tree/api"
	"github.com/sdcio/data-server/pkg/tree/types"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func GetPathCompletions(ctx context.Context, entry api.Entry, toComplete string) []string {
	var toCompletePath *sdcpb.Path
	var err error

	cleanToComplete := toComplete
	keyPart := ""
	doKeyAction := false

	if strings.LastIndex(toComplete, "[") > strings.LastIndex(toComplete, "]") {
		cleanToComplete = toComplete[:strings.LastIndex(toComplete, "[")]
		keyPart = toComplete[strings.LastIndex(toComplete, "[")+1:]
		doKeyAction = true
	}

	toCompletePath, err = sdcpb.ParsePath(cleanToComplete)
	if err != nil {
		return nil
	}

	// if we end with a ] meaning a key level, we need to check if we have more keys to complete or if we can switch back to path completion
	if strings.HasSuffix(toComplete, "]") {
		completePathCopy := toCompletePath.DeepCopy()
		completePathCopy.Elem[len(toCompletePath.Elem)-1].Key = nil

		elem, _ := NavigateSdcpbPath(ctx, entry, completePathCopy)
		if elem == nil {
			return nil
		}
		// if the schema element defines more keys then we have already, we need to continue key completion
		if len(GetSchemaKeys(elem)) > len(toCompletePath.Elem[len(toCompletePath.Elem)-1].Key) {
			// we have more keys to process, so we need to do key completion instead of path completion
			doKeyAction = true
		}
	}

	if doKeyAction {
		if strings.Contains(keyPart, "=") {
			return completeKey(ctx, entry, toCompletePath, keyPart)
		}
		return completeKeyName(ctx, entry, toCompletePath, keyPart)
	}

	return completePathName(ctx, entry, toCompletePath)
}

func completeKey(ctx context.Context, entry api.Entry, toCompletePath *sdcpb.Path, leftover string) []string {
	attrName, attrVal, _ := strings.Cut(leftover, "=")

	entry, err := NavigateSdcpbPath(ctx, entry, toCompletePath)
	if err != nil {
		return nil
	}

	lastLevelKeys := toCompletePath.Elem[len(toCompletePath.Elem)-1].Key

	if entry.GetSchema() == nil {
		entry, _ = GetFirstAncestorWithSchema(entry)
	}

	childs, err := FilterChilds(entry, lastLevelKeys)
	if err != nil {
		return nil
	}
	result := []string{}
	for _, e := range childs {
		em := e.GetChilds(types.DescendMethodActiveChilds)
		lv := em[attrName].GetLeafVariants().GetHighestPrecedence(false, true, false)

		elemVal := lv.Update.Value().ToString()
		if !strings.HasPrefix(elemVal, attrVal) {
			continue
		}
		newPath := toCompletePath.DeepCopy()
		if newPath.Elem[len(newPath.Elem)-1].Key == nil {
			newPath.Elem[len(newPath.Elem)-1].Key = map[string]string{}
		}
		newPath.Elem[len(newPath.Elem)-1].Key[attrName] = elemVal
		pstring := newPath.ToXPath(false)
		result = append(result, pstring)
	}
	return result
}
func completeKeyName(ctx context.Context, entry api.Entry, toCompletePath *sdcpb.Path, leftOver string) []string {
	toCompletePathCopy := toCompletePath.DeepCopy()
	existingKeys := map[string]struct{}{}
	for k := range toCompletePathCopy.Elem[len(toCompletePathCopy.Elem)-1].Key {
		existingKeys[k] = struct{}{}
	}

	toCompletePathCopy.Elem[len(toCompletePathCopy.Elem)-1].Key = nil
	entry, err := NavigateSdcpbPath(ctx, entry, toCompletePathCopy)
	if err != nil {
		return nil
	}
	result := []string{}
	for _, k := range GetSchemaKeys(entry) {
		_, keyexists := existingKeys[k]
		if strings.HasPrefix(k, leftOver) && !keyexists {
			result = append(result, fmt.Sprintf("%s[%s=", toCompletePath.ToXPath(false), k))
		}
	}
	return result
}
func completePathName(ctx context.Context, entry api.Entry, toCompletePath *sdcpb.Path) []string {
	var err error

	var incompleteLastElem *sdcpb.PathElem
	if len(toCompletePath.Elem) > 0 {
		// check if the provied path points to something that exists
		_, err := NavigateSdcpbPath(ctx, entry, toCompletePath)
		if err != nil {
			// path does not exist, so lets strip last elem
			if len(toCompletePath.Elem[len(toCompletePath.Elem)-1].Key) > 0 {
				// processing keys
			} else {
				// processing normal path elements
				// remove the last element since it is probably just partial
				incompleteLastElem = toCompletePath.Elem[len(toCompletePath.Elem)-1]
				toCompletePath.Elem = toCompletePath.Elem[:len(toCompletePath.Elem)-1]
			}
		}
	}

	entry, err = NavigateSdcpbPath(ctx, entry, toCompletePath)
	if err != nil {
		return nil
	}
	childs := entry.GetChilds(types.DescendMethodActiveChilds)

	var resultEntries []api.Entry
	doAdd := true
	for k, v := range childs {
		if incompleteLastElem != nil {
			doAdd = strings.HasPrefix(k, incompleteLastElem.Name)
		}
		if doAdd {
			resultEntries = append(resultEntries, v)
		}
	}

	results := make([]string, 0, len(resultEntries))
	//convert to xpath
	for _, e := range resultEntries {
		sdcpbPath := e.SdcpbPath()
		if len(GetSchemaKeys(e)) > 0 {
			results = append(results, fmt.Sprintf("%s[%s=", sdcpbPath.ToXPath(false), GetSchemaKeys(e)[0]))
		}
		results = append(results, fmt.Sprintf("%s", sdcpbPath.ToXPath(false)))
	}
	sort.Strings(results)
	return results
}
