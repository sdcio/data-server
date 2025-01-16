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
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/tree"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func (d *Datastore) populateTreeWithRunning(ctx context.Context, tc *tree.TreeContext, r *tree.RootEntry) error {
	upds, err := tc.ReadRunningFull(ctx)
	if err != nil {
		return err
	}

	for _, upd := range upds {
		newUpd := cache.NewUpdate(upd.GetPath(), upd.Bytes(), tree.RunningValuesPrio, tree.RunningIntentName, 0)
		_, err := r.AddCacheUpdateRecursive(ctx, newUpd, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Datastore) populateTree(ctx context.Context, req *sdcpb.SetIntentRequest, tc *tree.TreeContext) (r *tree.RootEntry, err error) {
	// create a new Tree
	root, err := tree.NewTreeRoot(ctx, tc)
	if err != nil {
		return nil, err
	}

	// read all the keys from the cache intended store but just the keys, no values are populated
	storeIndex, err := d.readStoreKeysMeta(ctx, cachepb.Store_INTENDED)
	if err != nil {
		return nil, err
	}
	tc.SetStoreIndex(storeIndex)

	converter := utils.NewConverter(d.getValidationClient())

	// list of updates to be added to the cache
	// Expands the value, in case of json to single typed value updates
	expandedReqUpdates, err := converter.ExpandUpdates(ctx, req.GetUpdate(), true)
	if err != nil {
		return nil, err
	}

	// temp storage for cache.Update of the req. They are to be added later.
	newCacheUpdates := make([]*cache.Update, 0, len(expandedReqUpdates))

	// Set of pathKeySet that need to be retrieved from the cache
	pathKeySet := tree.NewPathSet()

	for _, u := range expandedReqUpdates {
		pathslice, err := utils.CompletePath(nil, u.GetPath())
		if err != nil {
			return nil, err
		}

		pathKeySet.AddPath(pathslice)

		// since we already have the pathslice, we construct the cache.Update, but keep it for later
		// addition to the tree. First we need to mark the existing once for deltion

		// make sure typedValue is carrying the correct type
		err = d.validateUpdate(ctx, u)
		if err != nil {
			return nil, err
		}

		// convert value to []byte for cache insertion
		val, err := proto.Marshal(u.GetValue())
		if err != nil {
			return nil, err
		}

		// construct the cache.Update
		newCacheUpdates = append(newCacheUpdates, cache.NewUpdate(pathslice, val, req.GetPriority(), req.GetIntent(), 0))
	}

	root.LoadIntendedStoreOwnerData(ctx, req.GetIntent(), pathKeySet)

	// now add the cache.Updates from the actual request, after marking the old once for deletion.
	for _, upd := range newCacheUpdates {
		// add the cache.Update to the tree
		_, err = root.AddCacheUpdateRecursive(ctx, upd, true)
		if err != nil {
			return nil, err
		}
	}

	return root, nil
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
func (d *Datastore) SetIntentUpdate(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) (*sdcpb.SetIntentResponse, error) {
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

	// PH1: go through all updates from the intent to figure out
	// if they need to be applied based on the intent priority.
	logger.Debugf("reading intent paths to be updated from intended store; looking for the highest priority values")

	treeCacheSchemaClient := tree.NewTreeSchemaCacheClient(d.Name(), d.cacheClient, d.getValidationClient())
	tc := tree.NewTreeContext(treeCacheSchemaClient, req.GetIntent())

	root, err := d.populateTree(ctx, req, tc)
	if err != nil {
		return nil, err
	}

	err = d.populateTreeWithRunning(ctx, tc, root)
	if err != nil {
		return nil, err
	}

	logger.Debugf("finish insertion phase")
	root.FinishInsertionPhase()

	// perform validation
	// we use a channel and cumulate all the errors
	validationErrors := []error{}
	validationErrChan := make(chan error)

	validationWarnings := []error{}
	validationWarningsChan := make(chan error)

	wg := sync.WaitGroup{}

	go func() {
		root.Validate(ctx, validationErrChan, validationWarningsChan, true)
		close(validationErrChan)
		close(validationWarningsChan)
	}()

	wg.Add(1)
	go func() {
		// read from the Error channel
		for e := range validationErrChan {
			validationErrors = append(validationErrors, e)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		// read from the Warnings channel
		for e := range validationWarningsChan {
			validationWarnings = append(validationWarnings, e)
		}
		wg.Done()
	}()

	wg.Wait()
	logger.Tracef("Tree after Validate:%s\n", root.String())

	// check if errors are received
	// If so, join them and return the cumulated errors
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("cumulated validation errors:\n%v", errors.Join(validationErrors...))
	}

	if len(validationWarnings) > 0 {
		logger.Warnf("cumulated validation warnings:\n%v", errors.Join(validationWarnings...))
		// adding to response later on, when response struct is created
	}

	logger.Info("intent is valid")

	// retrieve the data that is meant to be send southbound (towards the device)
	updates := root.GetHighestPrecedence(true)
	deletes, err := root.GetDeletes(true)
	if err != nil {
		return nil, err
	}

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

	// add all the highes priority updates to the setDataReq
	for _, u := range updates {
		sdcpbUpd, err := d.cacheUpdateToUpdate(ctx, u.Update)
		if err != nil {
			return nil, err
		}
		setDataReq.Update = append(setDataReq.Update, sdcpbUpd)
	}

	// add all the deletes to the setDataReq
	for _, u := range deletes {
		p, err := u.SdcpbPath()
		if err != nil {
			return nil, err
		}
		setDataReq.Delete = append(setDataReq.Delete, p)
	}

	logger.Debug(prototext.Format(setDataReq))

	// set the response data indicationg the changes to the device
	setIntentResponse := &sdcpb.SetIntentResponse{
		Update: append(setDataReq.Update, setDataReq.Replace...),
		Delete: setDataReq.GetDelete(),
	}

	// populate response with validation warnings
	for _, e := range validationWarnings {
		setIntentResponse.Warnings = append(setIntentResponse.Warnings, e.Error())
	}

	// if it is a dry run, return now, skipping updating the device or the cache
	if req.DryRun {
		return setIntentResponse, nil
	}

	logger.Info("intent setting into candidate")
	// set the candidate
	_, err = d.setCandidate(ctx, setDataReq, false)
	if err != nil {
		return nil, err
	}

	// only if not the OnlyIntended flag is set, we transact to the device
	if !req.Delete || req.Delete && !req.OnlyIntended {
		logger.Info("intent set into candidate")
		// apply the resulting config to the device
		dataResp, err := d.applyIntent(ctx, candidateName, root)
		if err != nil {
			return nil, err
		}
		setIntentResponse.Warnings = append(setIntentResponse.Warnings, dataResp.GetWarnings()...)

		log.Infof("ds=%s intent=%s: intent applied", req.GetName(), req.GetIntent())
	}

	/////////////////////////////////////
	// update intent in intended store //
	/////////////////////////////////////

	// retrieve the data that is meant to be send towards the cache
	updatesOwner := root.GetUpdatesForOwner(req.GetIntent())
	deletesOwner := root.GetDeletesForOwner(req.GetIntent())

	// logging
	strSl := tree.Map(updates.ToCacheUpdateSlice(), func(u *cache.Update) string { return u.String() })
	logger.Debugf("Updates\n%s", strings.Join(strSl, "\n"))

	delSl := make(tree.PathSlices, 0, len(deletes))
	for _, del := range deletes {
		delSl = append(delSl, del.Path())
	}
	logger.Debugf("Deletes:\n%s", strings.Join(strSl, "\n"))

	strSl = tree.Map(updatesOwner, func(u *cache.Update) string { return u.String() })
	logger.Debugf("Updates Owner:\n%s", strings.Join(strSl, "\n"))

	strSl = deletesOwner.StringSlice()
	logger.Debugf("Deletes Owner:\n%s", strings.Join(strSl, "\n"))

	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, deletesOwner.ToStringSlice(), updatesOwner)
	if err != nil {
		return nil, fmt.Errorf("failed updating the intended store for %s: %w", d.Name(), err)
	}

	// fast and optimistic writeback to the config store
	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store: cachepb.Store_CONFIG,
	}, delSl.ToStringSlice(), updates.ToCacheUpdateSlice())
	if err != nil {
		return nil, fmt.Errorf("failed updating the running config store for %s: %w", d.Name(), err)
	}

	switch req.Delete {
	case true:
		err = d.deleteRawIntent(ctx, req.GetIntent(), req.GetPriority())
		if err != nil {
			return nil, err
		}
	case false:
		// The request intent is also stored in the cache
		// in the format it was received in
		err = d.saveRawIntent(ctx, req.GetIntent(), req)
		if err != nil {
			return nil, err
		}
	}

	logger.Infof("ds=%s intent=%s: intent saved", req.GetName(), req.GetIntent())
	return setIntentResponse, nil
}

func pathIsKeyAsLeaf(p *sdcpb.Path) bool {
	numPElem := len(p.GetElem())
	if numPElem < 2 {
		return false
	}

	_, ok := p.GetElem()[numPElem-2].GetKey()[p.GetElem()[numPElem-1].GetName()]
	return ok
}

func (d *Datastore) readStoreKeysMeta(ctx context.Context, store cachepb.Store) (map[string]tree.UpdateSlice, error) {
	entryCh, err := d.cacheClient.GetKeys(ctx, d.config.Name, store)
	if err != nil {
		return nil, err
	}

	result := map[string]tree.UpdateSlice{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-entryCh:
			if !ok {
				return result, nil
			}
			key := strings.Join(e.GetPath(), tree.KeysIndexSep)
			_, exists := result[key]
			if !exists {
				result[key] = tree.UpdateSlice{}
			}
			result[key] = append(result[key], e)
		}
	}
}
