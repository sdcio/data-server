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

	// set request to be applied into the candidate
	setDataReq := &sdcpb.SetDataRequest{
		Name: req.GetName(),
		Datastore: &sdcpb.DataStore{
			Type:     sdcpb.Type_CANDIDATE,
			Name:     candidateName,
			Owner:    req.GetIntent(),
			Priority: req.GetPriority(),
		},
		Update: make([]*sdcpb.Update, 0),
		Delete: make([]*sdcpb.Path, 0),
	}

	keys, err := d.readIntendedStoreKeys(ctx)
	if err != nil {
		return err
	}

	// create a new Tree
	root := tree.NewTreeRoot(d.getValidationClient())

	// Get all entries of the already existing intent
	allCurrentCacheEntries := d.readNewUpdatesHighestPriority(ctx, keys)

	// add all the existing entries
	for _, e := range allCurrentCacheEntries {
		for _, x := range e {
			root.AddCacheUpdateRecursive(x, false)
		}
	}

	// // Add Intended content
	// currentCacheEntries := d.cacheClient.Read(ctx, d.Name(),
	// 	&cache.Opts{
	// 		Store:         cachepb.Store_INTENDED,
	// 		PriorityCount: 2,
	// 	}, ic.newCompletePaths,
	// 	0)

	// // add all the existing entries
	// for _, u := range currentCacheEntries {
	// 	root.AddCacheUpdateRecursive(u, false)
	// }

	// Mark all the entries that belong to the owner / intent as deleted.
	// This is to allow for intent updates. We mark all existing entries for deletion up front.
	root.MarkOwnerDelete(req.GetIntent())

	// list of updates to be added to the cache later on.
	expandedReqUpdates, err := d.expandUpdates(ctx, req.GetUpdate(), true)
	if err != nil {
		return err
	}

	for _, upd := range expandedReqUpdates {

		// make sure typedValue is carrying the correct type
		err := d.validateUpdate(ctx, upd)
		if err != nil {
			return err
		}

		val, err := proto.Marshal(upd.GetValue())
		if err != nil {
			return err
		}
		pathSlice, err := utils.CompletePath(nil, upd.GetPath())
		if err != nil {
			return err
		}
		cUpd := cache.NewUpdate(pathSlice, val, req.GetPriority(), req.GetIntent(), int64(5))

		err = root.AddCacheUpdateRecursive(cUpd, true)
		if err != nil {
			return err
		}
	}

	// populate schema
	err = root.Walk(tree.TreeWalkerSchemaRetriever(ctx, d.getValidationClient()))
	if err != nil {
		return err
	}

	fmt.Printf("Tree:%s", root.String())

	updates := root.GetHighesPrio()

	fmt.Println("highes Prio Updates:")
	for _, u := range updates {
		fmt.Printf("Update: %v\n", u)
	}

	fmt.Printf("Updates of Owner %q:\n", req.GetIntent())
	updatesOwner := tree.LeafEntriesToCacheUpdates(root.GetByOwnerFiltered(req.GetIntent(), tree.FilterNonDeleted))
	for _, u := range updatesOwner {
		fmt.Printf("Update: %v\n", u)
	}

	fmt.Println("Deletes:")
	deletes := root.GetDeletes()
	for _, d := range deletes {
		fmt.Printf("Delete: %v\n", d)
	}

	// retrieve all the owner based entries that are marked as delete
	deletesOwnerUpdates := tree.LeafEntriesToCacheUpdates(root.GetByOwnerFiltered(req.GetIntent(), tree.FilterDeleted))
	// they are collected as cache.update, we just need the path
	deletesOwner := make([][]string, 0, len(deletesOwnerUpdates))
	// so collect the paths
	for _, d := range deletesOwnerUpdates {
		deletesOwner = append(deletesOwner, d.GetPath())
	}

	// PH1: go through all updates from the intent to figure out
	// if they need to be applied based on the intent priority.
	logger.Debugf("reading intent paths to be updated from intended store; looking for the highest priority values")

	// add all the updates to the setDataReq
	for _, u := range updates {
		sdcpbUpd, err := d.cacheUpdateToUpdate(ctx, u)
		if err != nil {
			return err
		}
		setDataReq.Update = append(setDataReq.Update, sdcpbUpd)
	}

	for _, u := range deletes {
		sdcpbUpd, err := d.cacheUpdateToUpdate(ctx, cache.NewUpdate(u, []byte{}, req.Priority, req.Intent, 0))
		if err != nil {
			return err
		}
		setDataReq.Delete = append(setDataReq.Delete, sdcpbUpd.GetPath())
	}

	fmt.Println(prototext.Format(setDataReq))

	log.Info("intent setting into candidate")
	_, err = d.setCandidate(ctx, setDataReq, false)
	if err != nil {
		return err
	}
	log.Info("intent set into candidate")
	// apply intent
	err = d.applyIntent(ctx, candidateName, req.GetPriority(), setDataReq)
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
	// replace intent in metadata store
	err = d.saveRawIntent(ctx, req.GetIntent(), req)
	if err != nil {
		return err
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
