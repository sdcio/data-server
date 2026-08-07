// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package configserver

import (
	"encoding/json"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// mergeConfigBlobs assembles a Document's flat path+value entries into a
// single nested, JSON_IETF-shaped map — the same shape
// importer/json.JsonTreeImporter already knows how to traverse, per the
// ADR's field-mapping table ("Value payload" row). List path elements (those
// carrying keys) become list entries (map[string]any, pre-seeded with their
// key leaves) inside a []any keyed by their element name, found-or-created
// by matching keys so repeated blobs under the same list entry converge on
// one map.
func mergeConfigBlobs(blobs []*ConfigBlob) (map[string]any, error) {
	root := map[string]any{}
	for _, b := range blobs {
		p, err := sdcpb.ParsePath(b.Path)
		if err != nil {
			return nil, fmt.Errorf("parsing path %q: %w", b.Path, err)
		}

		var val any
		if len(b.Value) > 0 {
			if err := json.Unmarshal(b.Value, &val); err != nil {
				return nil, fmt.Errorf("unmarshalling value at %q: %w", b.Path, err)
			}
		}

		if err := insertAtPath(root, p.GetElem(), val); err != nil {
			return nil, fmt.Errorf("inserting %q: %w", b.Path, err)
		}
	}
	return root, nil
}

// insertAtPath walks elems into node, creating intermediate containers/list
// entries as needed, and sets val at the final element.
func insertAtPath(node map[string]any, elems []*sdcpb.PathElem, val any) error {
	if len(elems) == 0 {
		return fmt.Errorf("empty path")
	}
	elem := elems[0]
	rest := elems[1:]

	if len(elem.GetKey()) == 0 {
		if len(rest) == 0 {
			node[elem.Name] = val
			return nil
		}
		child, ok := node[elem.Name].(map[string]any)
		if !ok {
			child = map[string]any{}
			node[elem.Name] = child
		}
		return insertAtPath(child, rest, val)
	}

	entry := findOrCreateListEntry(node, elem)
	if len(rest) == 0 {
		// A keyed element with nothing after it has only its own key leaves
		// to contribute — already seeded by findOrCreateListEntry.
		return nil
	}
	return insertAtPath(entry, rest, val)
}

// findOrCreateListEntry returns the list entry under node[elem.Name] whose
// key leaves match elem's keys, creating (and appending) one pre-seeded with
// those key leaves if none matches yet.
func findOrCreateListEntry(node map[string]any, elem *sdcpb.PathElem) map[string]any {
	list, _ := node[elem.Name].([]any)
	for _, e := range list {
		if entry, ok := e.(map[string]any); ok && listEntryMatchesKeys(entry, elem.GetKey()) {
			return entry
		}
	}

	entry := map[string]any{}
	for k, v := range elem.GetKey() {
		entry[k] = v
	}
	node[elem.Name] = append(list, entry)
	return entry
}

func listEntryMatchesKeys(entry map[string]any, keys map[string]string) bool {
	for k, v := range keys {
		ev, ok := entry[k]
		if !ok || fmt.Sprintf("%v", ev) != v {
			return false
		}
	}
	return true
}
