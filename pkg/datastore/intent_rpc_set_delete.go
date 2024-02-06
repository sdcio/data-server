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
	"strings"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/utils"
)

func (d *Datastore) SetIntentDelete(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) error {
	log.Debugf("set intent delete start")
	defer log.Debugf("set intent  delete end")

	ic, err := d.newIntentDeleteContext(ctx, req)
	if err != nil {
		return err
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
		Delete: make([]*sdcpb.Path, 0),
		Update: make([]*sdcpb.Update, 0),
	}

	for idx, cp := range ic.appliedCompletePaths {
		currentCacheEntries, ok := ic.current2HighestCacheEntriesPerPath[strings.Join(cp, ",")]
		if !ok {
			continue
		}
		log.Debugf("ds=%s | %s | path=%v has %d entry(ies) layers", d.Name(), req.GetIntent(), cp, len(currentCacheEntries))
		for i, cce := range currentCacheEntries {
			log.Debugf("ds=%s | %s | path=%v has entry.%d %v", d.Name(), req.GetIntent(), cp, i, cce)
			for j, cc := range cce {
				log.Debugf("ds=%s | %s | path=%v has entry.%d.%d %v", d.Name(), req.GetIntent(), cp, i, j, cc)
			}
		}

		if currentCacheEntries == nil || currentCacheEntries[0] == nil {
			log.Warningf("unexpected empty response from cache for path: %v", cp)
			continue
		}
		// the intent has a lower priority than the highest one for this path
		// => nothing to change, but ?? add the highest intent entry for validation
		if currentCacheEntries[0][0].Priority() < req.GetPriority() {
			upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0][0])
			if err != nil {
				return err
			}
			setDataReq.Update = append(setDataReq.Update, upd)
			continue
		}
		switch len(currentCacheEntries) {
		case 0: // should not happen
		case 1: // single priority
			// if the current intent is the first (ts sorted), delete it.
			if currentCacheEntries[0][0].Owner() == req.GetIntent() {
				isKeyAsLeaf := pathIsKeyAsLeaf(ic.appliedPaths[idx])
				// if the path is a key and there is no next priority => delete the list
				if isKeyAsLeaf && len(currentCacheEntries[0]) == 1 {
					noKeyPath := &sdcpb.Path{Elem: ic.appliedPaths[idx].GetElem()[:len(ic.appliedPaths[idx].GetElem())-1]}
					setDataReq.Delete = append(setDataReq.Delete, noKeyPath)
					continue
				}
				// else delete the leaf
				setDataReq.Delete = append(setDataReq.Delete, ic.appliedPaths[idx])
				switch len(currentCacheEntries[0]) {
				case 0: // should not happen
				case 1: // this is own intent
				default: // apply next one
					upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0][1])
					if err != nil {
						return err
					}
					setDataReq.Update = append(setDataReq.Update, upd)
				}
			}
		default: // 2 priorities
			if currentCacheEntries[0][0].Owner() == req.GetIntent() {
				if pathIsKeyAsLeaf(ic.appliedPaths[idx]) {
					continue
				}
				setDataReq.Delete = append(setDataReq.Delete, ic.appliedPaths[idx])
				switch len(currentCacheEntries[0]) {
				case 0: // should not happen
				case 1: // this is own intent, apply next priority
					upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[1][0])
					if err != nil {
						return err
					}
					setDataReq.Update = append(setDataReq.Update, upd)
				default: // apply next one
					upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0][1])
					if err != nil {
						return err
					}
					setDataReq.Update = append(setDataReq.Update, upd)
				}
			}
		}
	}
	//// END OPT

	log.Debugf("ds=%s | %s | done building set data request for the candidate", d.Name(), req.GetIntent())
	log.Debugf("ds=%s | %s | set data request: START", d.Name(), req.GetIntent())
	for i, upd := range setDataReq.GetUpdate() {
		log.Debugf("ds=%s | %s | set data request update.%d: %v", d.Name(), req.GetIntent(), i, upd)
	}
	for i, del := range setDataReq.GetDelete() {
		log.Debugf("ds=%s | %s | set data request delete.%d: %v", d.Name(), req.GetIntent(), i, del)
	}
	log.Debugf("ds=%s | %s | set data request: END", d.Name(), req.GetIntent())
	// write to candidate
	log.Debugf("ds=%s | %s | set data into candidate: START", d.Name(), req.GetIntent())
	_, err = d.setCandidate(ctx, setDataReq, false)
	if err != nil {
		return err
	}
	log.Debugf("ds=%s | %s | set data into candidate: END", d.Name(), req.GetIntent())
	log.Debugf("ds=%s | %s | apply intent: START", d.Name(), req.GetIntent())
	// apply intent
	err = d.applyIntent(ctx, candidateName, req.GetPriority(), setDataReq)
	if err != nil {
		return err
	}
	log.Debugf("ds=%s | %s | apply intent: END", d.Name(), req.GetIntent())
	log.Debugf("ds=%s | %s | delete from intended store: START", d.Name(), req.GetIntent())
	// delete intent from intended store
	err = d.cacheClient.Modify(ctx, d.Config().Name, &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, ic.appliedCompletePaths, nil)
	if err != nil {
		return err
	}
	log.Debugf("ds=%s | %s | delete from intended store: END", d.Name(), req.GetIntent())
	// delete raw intent
	return d.deleteRawIntent(ctx, req.GetIntent(), req.GetPriority())
}

type intentDeleteContext struct {
	appliedCompletePaths               [][]string
	appliedPaths                       []*sdcpb.Path
	current2HighestCacheEntriesPerPath map[string][][]*cache.Update
}

func (d *Datastore) newIntentDeleteContext(ctx context.Context, req *sdcpb.SetIntentRequest) (*intentDeleteContext, error) {
	// get current intent notifications
	log.Debugf("ds=%s | %s | getting intent notifications", d.Name(), req.GetIntent())
	intentNotifications, err := d.getIntentFlatNotifications(ctx, req.GetIntent(), req.GetPriority())
	if err != nil {
		return nil, err
	}
	ic := &intentDeleteContext{
		appliedCompletePaths:               make([][]string, 0, len(intentNotifications)),
		appliedPaths:                       make([]*sdcpb.Path, 0, len(intentNotifications)),
		current2HighestCacheEntriesPerPath: map[string][][]*cache.Update{},
	}
	// build paths current intent paths
	for _, n := range intentNotifications {
		for i, upd := range n.GetUpdate() {
			log.Debugf("ds=%s | %s | has applied update.%d: %v", d.Name(), req.GetIntent(), i, upd)
			cp, err := utils.CompletePath(nil, upd.GetPath())
			if err != nil {
				return nil, err
			}
			ic.appliedCompletePaths = append(ic.appliedCompletePaths, cp)
			ic.appliedPaths = append(ic.appliedPaths, upd.GetPath())
		}
	}
	log.Debugf("ds=%s | %s | getting intent 2 highest priorities per path", d.Name(), req.GetIntent())
	ic.current2HighestCacheEntriesPerPath = d.readCurrentUpdatesHighestPriorities(ctx, ic.appliedCompletePaths, 2)
	return ic, nil
}
