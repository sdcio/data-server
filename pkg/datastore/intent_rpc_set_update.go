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

package datastore

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sdcio/cache/proto/cachepb"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils"
)

const (
	keysIndexSep = "_"
)

// populateTreeWithIntentData populates the given tree with intent data thats stored in the cache already
// Queries the Caches keys, filters out entries that belong to given owner and adds them to the given keys and keyindex structs.
// Finally queries the values for those entries, adds them to the given tree and marks the owners entries as deleted.
func (d *Datastore) populateTreeWithIntentData(ctx context.Context, root *tree.RootEntry, owner string, keyIndex map[string]struct{}, keys [][]string) error {

	// read all Intended Store Keys in their extednded format, containing
	// not just paths but also the owner and priority etc.
	// !!! Values will not be populated !!!
	keysMeta, err := d.readIntendedStoreKeysMeta(ctx)
	if err != nil {
		return err
	}

	// range through all the keys and filter out the once that
	// actually belong to the intent
	for _, keyMeta := range keysMeta {
		for _, k := range keyMeta {
			if k.Owner() == owner {
				// if the key is not yet listed in the keys slice, add it otherwise skip
				if _, exists := keyIndex[strings.Join(k.GetPath(), keysIndexSep)]; !exists {
					keys = append(keys, k.GetPath())
				}
			}
		}
	}

	// Get all entries of the already existing intent
	highesCurrentCacheEntries := d.readCurrentUpdatesHighestPriorities(ctx, keys, 2)

	// add all the existing entries
	for _, entrySlice := range highesCurrentCacheEntries {
		for _, v := range entrySlice {
			for _, x := range v {
				root.AddCacheUpdateRecursive(x, false)
			}
		}
	}

	// Mark all the entries that belong to the owner / intent as deleted.
	// This is to allow for intent updates. We mark all existing entries for deletion up front.
	root.MarkOwnerDelete(owner)

	return nil
}

// SetIntentUpdate Processes new and updated intents
//
// The main concept is as follows.
//  1. Get all keys from the cache along with the "metadata" (Owner, Priority, etc.) Note: Requesting the values is the expensive task with the default cache implementation
//  2. Filter the keys for entries that belong to the intent (Owner) which is necessary for updated intents (delete config entries that do no longer exist)
//  3. Calculate all the paths that the new intent request touches
//  4. Combine the keys from the two previous steps to query them from the cache just once.
//  5. Query the cache with the resulting keys to also get the values.
//  6. Add the received cache entries to the tree with the new-flag set to false.
//  7. Mark all entries in the tree for the specific owner as deleted.
//  8. Add all the new request entries to the tree with the new flag set to true. The tree will evaluate the values and adjust its internal state (new, deleted and updated)
//     for these entries. If the value remains unchanged, it will reset the new flag if it is a different value, it will set the updated flag and reset the delete flag.
//  9. The tree will be populated with schema information.
//  10. Now the tree can be queried for the highes priority values ".GetHighesPrio(true)". It will also consider the deleted flag and only return new or updated values.
//     This is the calculation the yields the updates that will need to be pushed to the device.
//  11. .GetDeletes() returns the entries that are still marked for deletion. The Paths will be extracted and then send to the device as deletes (path aggregation is
//     applied, if e.g. a whole interface is delted, the deleted paths only contains the delete for the interface, not all its leafs)
//  12. All updates (New & Updated) for the specifc owner / intent are being retrieved from the tree to update the cache.
//  13. All remaining deletes for the specifc owner / intent are being retrieved from the tree to remove them from the cache.
//  14. The request towards southbound is created with the device updates / deletes. A candidate is created, and applied to the device.
//  15. The owner based updates and deletes are being pushed into the cache.
//  16. The raw intent (as received in the req) is stored as a blob in the cache.
func (d *Datastore) SetIntentUpdate(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) error {
	logger := log.NewEntry(
		log.New()).WithFields(log.Fields{
		"ds":       d.Name(),
		"intent":   req.GetIntent(),
		"priority": req.GetPriority(),
	})
	logger.Logger.SetLevel(log.GetLevel())
	logger.Logger.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	logger.Debugf("set intent update start")
	defer logger.Debugf("set intent update end")

	// create a new Tree
	root := tree.NewTreeRoot()

	// list of updates to be added to the cache
	// Expands the value, in case of json to single typed value updates
	expandedReqUpdates, err := d.expandUpdates(ctx, req.GetUpdate(), true)
	if err != nil {
		return err
	}

	// temp storage for cache.Update of the req. They are to be added later.
	newCacheUpdates := make([]*cache.Update, 0, len(expandedReqUpdates))

	// stores the keys that are present in the existing intent,
	// that is already stored in the cache
	keys := make([][]string, 0, len(expandedReqUpdates))
	// index for already added keys
	keyIndex := map[string]struct{}{}

	for _, u := range expandedReqUpdates {
		pathslice, err := utils.CompletePath(nil, u.GetPath())
		if err != nil {
			return err
		}

		keys = append(keys, pathslice)
		keyIndex[strings.Join(pathslice, keysIndexSep)] = struct{}{}

		// since we already have the pathslice, we construct the cache.Update, but keep it for later
		// addition to the tree. First we need to mark the existing once for deltion

		// make sure typedValue is carrying the correct type
		err = d.validateUpdate(ctx, u)
		if err != nil {
			return err
		}

		// convert value to []byte for cache insertion
		val, err := proto.Marshal(u.GetValue())
		if err != nil {
			return err
		}

		// construct the cache.Update
		newCacheUpdates = append(newCacheUpdates, cache.NewUpdate(pathslice, val, req.GetPriority(), req.GetIntent(), 0))
	}

	// populate the tree with all the existing entries from the cache, that match relevant paths (paths from the old intent, and the new request).
	err = d.populateTreeWithIntentData(ctx, root, req.GetIntent(), keyIndex, keys)
	if err != nil {
		return err
	}

	// now add the cache.Updates from the actual request, after marking the old once for deletion.
	for _, upd := range newCacheUpdates {
		// add the cache.Update to the tree
		err = root.AddCacheUpdateRecursive(upd, true)
		if err != nil {
			return err
		}
	}

	// populate schema within the tree
	err = root.Walk(tree.TreeWalkerSchemaRetriever(ctx, d.getValidationClient()))
	if err != nil {
		return err
	}

	fmt.Printf("Tree:%s\n", root.String())

	// retrieve updates with highes priority
	// this is the config thats is to be pushed to the device
	updates := root.GetHighesPrio(true)

	fmt.Println("highes Prio Updates:")
	for _, u := range updates {
		fmt.Printf("Update: %v\n", u)
	}

	// Get the entries that are to be deleted from the device config
	// This is because a certain value was deleted and there is no
	// lower priority / shadowed value in the cache anymore
	deletes := root.GetDeletes()

	fmt.Println("Deletes:")
	for _, d := range deletes {
		fmt.Printf("Delete: %v\n", d)
	}

	// retrieve all the entries from the tree that belong to the given
	// Owner / Intent, skipping the once marked for deletion
	// this is to insert / update entries in the cache.
	updatesOwner := tree.LeafEntriesToCacheUpdates(root.GetByOwnerFiltered(req.GetIntent(), tree.FilterNonDeleted))

	fmt.Printf("Updates of Owner %q:\n", req.GetIntent())
	for _, u := range updatesOwner {
		fmt.Printf("Update: %v\n", u)
	}

	// retrieve all entries from the tree that belong to the given user
	// and that are marked for deletion.
	// This is to cover all the cases where an intent was changed and certain
	// part of the config got deleted.
	deletesOwnerUpdates := tree.LeafEntriesToCacheUpdates(root.GetByOwnerFiltered(req.GetIntent(), tree.FilterDeleted))
	// they are retrieved as cache.update, we just need the path for deletion from cache
	deletesOwner := make([][]string, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPath())
	}

	// PH1: go through all updates from the intent to figure out
	// if they need to be applied based on the intent priority.
	logger.Debugf("reading intent paths to be updated from intended store; looking for the highest priority values")

	// set request to be applied into the candidate
	setDataReq := &sdcpb.SetDataRequest{
		Name: req.GetName(),
		Datastore: &sdcpb.DataStore{
			Type:     sdcpb.Type_CANDIDATE,
			Name:     candidateName,
			Owner:    req.GetIntent(),
			Priority: req.GetPriority(),
		},
		Update: make([]*sdcpb.Update, 0, len(updates)),
		Delete: make([]*sdcpb.Path, 0, len(deletes)),
	}

	// add all the updates to the setDataReq
	for _, u := range updates {
		sdcpbUpd, err := d.cacheUpdateToUpdate(ctx, u)
		if err != nil {
			return err
		}
		setDataReq.Update = append(setDataReq.Update, sdcpbUpd)
	}

	// add all the deletes to the setDataReq
	for _, u := range deletes {
		sdcpbUpd, err := d.cacheUpdateToUpdate(ctx, cache.NewUpdate(u, []byte{}, req.Priority, req.Intent, 0))
		if err != nil {
			return err
		}
		setDataReq.Delete = append(setDataReq.Delete, sdcpbUpd.GetPath())
	}

	fmt.Println(prototext.Format(setDataReq))

	log.Info("intent setting into candidate")
	// set the candidate
	_, err = d.setCandidate(ctx, setDataReq, false)
	if err != nil {
		return err
	}
	log.Info("intent set into candidate")
	// apply the resulting config to the device
	err = d.applyIntent(ctx, candidateName, setDataReq)
	if err != nil {
		return err
	}
	logger.Debug()
	logger.Debug("intent is validated")
	log.Infof("ds=%s intent=%s: intent applied", req.GetName(), req.GetIntent())
	logger.Debug()

	/////////////////////////////////////
	// update intent in intended store //
	/////////////////////////////////////

	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, deletesOwner, updatesOwner)
	if err != nil {
		return err
	}

	switch req.Delete {
	case true:
		err = d.deleteRawIntent(ctx, req.GetIntent(), req.GetPriority())
		if err != nil {
			return err
		}
	case false:
		// The request intent is also stored in the cache
		// in the format it was received in
		err = d.saveRawIntent(ctx, req.GetIntent(), req)
		if err != nil {
			return err
		}
	}

	log.Infof("ds=%s intent=%s: intent saved", req.GetName(), req.GetIntent())
	return nil
}

func pathIsKeyAsLeaf(p *sdcpb.Path) bool {
	numPElem := len(p.GetElem())
	if numPElem < 2 {
		return false
	}

	_, ok := p.GetElem()[numPElem-2].GetKey()[p.GetElem()[numPElem-1].GetName()]
	return ok
}

func (d *Datastore) readIntendedPathHighestPriorities(ctx context.Context, cp []string, count uint64) [][]*cache.Update {
	currentCacheEntries := d.cacheClient.Read(ctx, d.Name(),
		&cache.Opts{
			Store:         cachepb.Store_INTENDED,
			PriorityCount: count,
		}, [][]string{cp},
		0)
	if len(currentCacheEntries) == 0 {
		return nil
	}

	groupping := make(map[int32][]*cache.Update)
	for _, cce := range currentCacheEntries {
		if _, ok := groupping[cce.Priority()]; !ok {
			groupping[cce.Priority()] = make([]*cache.Update, 0, 1)
		}
		groupping[cce.Priority()] = append(groupping[cce.Priority()], cce)
	}
	priorities := make([]int32, 0, count)
	for k := range groupping {
		priorities = append(priorities, k)
	}
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i] < priorities[j]
	})
	rs := make([][]*cache.Update, 0, count)
	for _, pr := range priorities {
		rs = append(rs, groupping[pr])
	}
	return rs
}

func (d *Datastore) getNextPriority(ctx context.Context, intentPriority int32, intentName string, cp []string) *cache.Update {
	ccu := d.readIntendedPathHighestPriorities(ctx, cp, 2)
	switch lccu := len(ccu); lccu {
	case 0:
		return nil
	case 1:
		// the update to be applied is the first update that's not owned by the current intent
		for _, cu := range ccu[0] {
			if cu.Owner() != intentName {
				return cu
			}
		}
		return nil
	default: // case 2:
		// check if the first received priority is the same as the intent's
		updatesSet := ccu[1]
		if ccu[0][0].Priority() != intentPriority {
			updatesSet = ccu[0] // use first updates set
		}
		for _, cu := range updatesSet {
			if cu.Owner() != intentName {
				return cu
			}
		}
		return nil
	}
}

func (d *Datastore) readIntendedStoreKeysMeta(ctx context.Context) (map[string][]*cache.Update, error) {
	entryCh, err := d.cacheClient.GetIntendedKeysMeta(ctx, d.config.Name)
	if err != nil {
		return nil, err
	}

	result := map[string][]*cache.Update{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-entryCh:
			if !ok {
				return result, nil
			}
			key := strings.Join(e.GetPath(), "/")
			_, exists := result[key]
			if !exists {
				result[key] = []*cache.Update{}
			}
			result[key] = append(result[key], e)
		}
	}
}

func (d *Datastore) readIntendedStoreKeys(ctx context.Context) ([][]string, error) {
	entryCh, err := d.cacheClient.GetIntendedKeys(ctx, d.config.Name)
	if err != nil {
		return nil, err
	}

	result := [][]string{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-entryCh:
			if !ok {
				return result, nil
			}
			result = append(result, e)
		}
	}
}

func (d *Datastore) readNewUpdatesHighestPriority(ctx context.Context, ccp [][]string) map[string][]*cache.Update {
	currentCacheEntries := d.cacheClient.Read(ctx, d.Name(),
		&cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, ccp,
		0)
	rs := make(map[string][]*cache.Update, len(ccp))

	for _, cce := range currentCacheEntries {
		sp := strings.Join(cce.GetPath(), ",")
		if _, ok := rs[sp]; !ok {
			rs[sp] = make([]*cache.Update, 0, 1)
		}
		rs[sp] = append(rs[sp], cce)
	}
	return rs
}

func (d *Datastore) readCurrentUpdatesHighestPriorities(ctx context.Context, ccp [][]string, count uint64) map[string][][]*cache.Update {
	currentCacheEntries := d.cacheClient.Read(ctx, d.Name(),
		&cache.Opts{
			Store:         cachepb.Store_INTENDED,
			PriorityCount: count,
		}, ccp,
		0)
	if len(currentCacheEntries) == 0 {
		return nil
	}
	rs := make(map[string][][]*cache.Update)
	groupings := make(map[string]map[int32][]*cache.Update)

	for _, cce := range currentCacheEntries {
		sp := strings.Join(cce.GetPath(), ",")
		if _, ok := rs[sp]; !ok {
			rs[sp] = make([][]*cache.Update, 0, 1)
		}
		if _, ok := groupings[sp]; !ok {
			groupings[sp] = make(map[int32][]*cache.Update)
		}
		if _, ok := groupings[sp][cce.Priority()]; !ok {
			groupings[sp][cce.Priority()] = make([]*cache.Update, 0, 1)
		}
		groupings[sp][cce.Priority()] = append(groupings[sp][cce.Priority()], cce)
	}
	for sp, groupping := range groupings {
		priorities := make([]int32, 0, count)
		for k := range groupping {
			priorities = append(priorities, k)
		}
		sort.Slice(priorities, func(i, j int) bool {
			return priorities[i] < priorities[j]
		})
		for _, pr := range priorities {
			rs[sp] = append(rs[sp], groupping[pr])
		}
	}
	//
	return rs
}
