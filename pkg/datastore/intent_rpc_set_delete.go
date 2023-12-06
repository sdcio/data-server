package datastore

import (
	"context"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/utils"
)

func (d *Datastore) SetIntentDelete(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) error {
	log.Debugf("set intent delete start")
	defer log.Debugf("set intent  delete end")

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
	// get current intent notifications
	intentNotifications, err := d.getIntentFlat(ctx, req.GetIntent())
	if err != nil {
		return err
	}

	// get paths of the current intent
	appliedCompletePaths := make([][]string, 0, len(intentNotifications))
	appliedPaths := make([]*sdcpb.Path, 0, len(intentNotifications))
	for _, n := range intentNotifications {
		for i, upd := range n.GetUpdate() {
			log.Debugf("ds=%s | %s | has applied update.%d: %v", d.Name(), req.GetIntent(), i, upd)
			cp, err := utils.CompletePath(nil, upd.GetPath())
			if err != nil {
				return err
			}
			appliedCompletePaths = append(appliedCompletePaths, cp)
			appliedPaths = append(appliedPaths, upd.GetPath())
		}
	}

	// go through applied paths and check if they are the highest priority
	for idx, cp := range appliedCompletePaths {
		// get the current highest priority value for this path
		currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name, &cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, [][]string{cp}, 0)
		log.Debugf("ds=%s | %s | highest update (p=updates): %v=%v", d.Name(), req.GetIntent(), cp, currentCacheEntries)
		switch len(currentCacheEntries) {
		case 0:
			// should not happen
			// panic(currentCacheEntries)
		case 1:
			if currentCacheEntries[0].Owner() == req.GetIntent() {
				if pathIsKeyAsLeaf(appliedPaths[idx]) {
					continue
				}
				setDataReq.Delete = append(setDataReq.Delete, appliedPaths[idx])
			}
		default:
			// should be taken care of after delete from intended
		}

	}
	// delete intent from intended store
	err = d.cacheClient.Modify(ctx, d.Config().Name, &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, appliedCompletePaths, nil)
	if err != nil {
		return err
	}

	// This defer function writes back the intent notification
	// in the intended store if one of the following steps fail:
	// - write to candidate and initial validations
	// - apply intent: further validations and send to southbound
	// assume will fail
	var failed = true
	defer func() {
		if !failed {
			return
		}
		log.Debugf("ds: %s: intent %s: reverting back intended notifications", d.Name(), req.GetIntent())
		cacheUpdates := make([]*cache.Update, 0, len(intentNotifications))
		for _, n := range intentNotifications {
			for _, upd := range n.GetUpdate() {
				cup, err := d.cacheClient.NewUpdate(upd)
				if err != nil { // this should not fail
					log.Errorf("failed to revert the intent back: %v", err)
					return
				}
				cacheUpdates = append(cacheUpdates, cup)
			}
		}
		err := d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
			Store:    cachepb.Store_INTENDED,
			Owner:    req.GetIntent(),
			Priority: req.GetPriority(),
		}, nil, cacheUpdates)
		if err != nil {
			log.Errorf("failed to revert the intent back: %v", err)
		}
	}()

	// add delete paths
	// setDataReq.Delete = d.buildDeletePaths(ctx, req.GetPriority(), req.GetIntent(), setDataReq.Delete)

	// go through the applied paths again and get the highest priority (path,value) after deletion
	for idx, cp := range appliedCompletePaths {
		currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name, &cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, [][]string{cp}, 0)
		log.Debugf("ds=%s | %s | highest update after delete (p=updates): %v=%v", d.Name(), req.GetIntent(), cp, currentCacheEntries)
		switch lcce := len(currentCacheEntries); lcce {
		case 0:
			// no next highest
			p := appliedPaths[idx]
			if pathIsKeyAsLeaf(p) {
				noKeyPath := &sdcpb.Path{Elem: p.GetElem()[:len(p.GetElem())-1]}
				setDataReq.Delete = append(setDataReq.Delete, noKeyPath)
			}
		case 1:
			// there is a next highest
			// fmt.Println("there is a next highest: ", currentCacheEntries[0])
			log.Debugf("ds=%s | %s | path %v next highest is %v", d.Name(), req.GetIntent(), cp, currentCacheEntries[0])
			upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
			if err != nil {
				return err
			}
			// check if it's a key leaf, in which case don't add it to the update
			if pathIsKeyAsLeaf(upd.GetPath()) {
				continue
			}
			// numPElem := len(upd.GetPath().GetElem())
			// lastPe := upd.GetPath().GetElem()[numPElem-1]
			// if _, ok := upd.GetPath().GetElem()[numPElem-2].GetKey()[lastPe.GetName()]; ok {
			// 	continue
			// }
			log.Debugf("ds=%s | %s | found highest update after delete: %v", d.Name(), req.GetIntent(), upd)
			setDataReq.Update = append(setDataReq.Update, upd)
		default:
			// there is multiple next highest
			log.Debugf("ds=%s | %s | path %v has %d next highest", d.Name(), req.GetIntent(), cp, lcce)
			for i, cce := range currentCacheEntries {
				log.Debugf("ds=%s | %s | path %v next highest idx=%d is %v", d.Name(), req.GetIntent(), cp, i, cce)
			}
			// select last one by ts
			upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[lcce-1])
			if err != nil {
				return err
			}
			// check if it's key leaf, in which case don't add it to the update
			numPElem := len(upd.GetPath().GetElem())
			lastPe := upd.GetPath().GetElem()[numPElem-1]
			if _, ok := upd.GetPath().GetElem()[numPElem-2].GetKey()[lastPe.GetName()]; ok {
				continue
			}
			setDataReq.Update = append(setDataReq.Update, upd)
		}
	}

	//
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
	_, err = d.Set(ctx, setDataReq)
	if err != nil {
		return err
	}

	// apply intent
	err = d.applyIntent(ctx, candidateName, req.GetPriority(), setDataReq)
	if err != nil {
		return err
	}
	failed = false
	// delete raw intent
	return d.deleteRawIntent(ctx, req.GetIntent())
}

// func (d *Datastore) buildDeletePaths(ctx context.Context, priority int32, owner string, appliedPaths []*sdcpb.Path) []*sdcpb.Path {
// 	log.Debugf("ds=%s | %s | start buildDeletePaths", d.Name(), owner)
// 	defer log.Debugf("ds=%s | %s | end buildDeletePaths", d.Name(), owner)
// 	// sort paths by length
// 	sort.Slice(appliedPaths, func(i, j int) bool {
// 		return len(appliedPaths[i].GetElem()) < len(appliedPaths[j].GetElem())
// 	})

// 	// this map keeps track of paths added as deletes
// 	added := make(map[string]struct{})
// 	deletePaths := make([]*sdcpb.Path, 0, len(appliedPaths))

// 	for _, p := range appliedPaths {
// 		xpAdd := utils.ToXPath(p, false)
// 		if _, ok := added[xpAdd]; ok {
// 			continue
// 		}
// 		log.Debugf("ds=%s | %s | path=%v is a delete path", d.Name(), owner, p)
// 		added[xpAdd] = struct{}{}
// 		deletePaths = append(deletePaths, p)

// 		log.Debugf("ds=%s | %s | path=%v | looking for keys", d.Name(), owner, p)
// 		// check if the path contains lists (i.e keys)
// 		for idx, pe := range p.GetElem() {
// 			if len(pe.GetKey()) == 0 {
// 				continue
// 			}
// 			log.Debugf("ds=%s | %s | xx | PE %s has key %v", d.Name(), owner, pe.Name, pe.Key)
// 			// path has keys
// 			//
// 			for k, v := range pe.GetKey() {
// 				_ = v
// 				keyPath := &sdcpb.Path{
// 					Elem: make([]*sdcpb.PathElem, idx+1),
// 				}
// 				for i := 0; i < idx+1; i++ {
// 					keyPath.Elem[i] = &sdcpb.PathElem{
// 						Name: p.GetElem()[i].GetName(),
// 						Key:  copyMap(p.GetElem()[i].GetKey()),
// 					}
// 				}
// 				// keyPath := &sdcpb.Path{Elem: p.GetElem()[:i+1]}
// 				keyPath.Elem = append(keyPath.Elem, &sdcpb.PathElem{Name: k})

// 				// ignoring error because there is no prefix
// 				kcp, _ := utils.CompletePath(nil, keyPath)
// 				log.Debugf("ds=%s | %s | checking path %v in intended store", d.Name(), owner, kcp)

// 				keyCacheUpdate := d.cacheClient.Read(ctx, d.Name(), &cache.Opts{
// 					Store: cachepb.Store_INTENDED,
// 				}, [][]string{kcp}, 0)
// 				log.Debugf("ds=%s | %s | path %v found update %v", d.Name(), owner, kcp, keyCacheUpdate)
// 				switch len(keyCacheUpdate) {
// 				case 0:
// 					// does not exist, add as delete path anyways?
// 				case 1:
// 					log.Debugf("ds=%s | %s | path %v found update %v", d.Name(), owner, kcp, keyCacheUpdate[0])
// 					log.Debugf("ds=%s | %s | path %v found update | intent priority %d, cachedupdate %s/%d", d.Name(), owner, kcp, priority, keyCacheUpdate[0].Owner(), keyCacheUpdate[0].Priority())
// 					switch {
// 					case priority < keyCacheUpdate[0].Priority():
// 						// list item held by an intent with a "lower" priority
// 						// keep going to add as a delete path
// 					case priority == keyCacheUpdate[0].Priority():
// 						// list item held by an intent with an equal priority
// 						// check owner
// 						if keyCacheUpdate[0].Owner() != owner {
// 							// not same owner go to next key
// 							continue // go to next key
// 						}
// 						// keep going to add as a delete path
// 					case priority > keyCacheUpdate[0].Priority():
// 						// list item held by an intent with an "higher" priority
// 						continue // go to next key
// 					}
// 				default:
// 					log.Debugf("ds=%s | %s | path %v found update %v", d.Name(), owner, kcp, keyCacheUpdate[0])
// 					log.Debugf("ds=%s | %s | path %v found update | intent priority %d, cachedUpdate %s/%d", d.Name(), owner, kcp, priority, keyCacheUpdate[0].Owner(), keyCacheUpdate[0].Priority())
// 					switch {
// 					case priority < keyCacheUpdate[0].Priority():
// 						// list item held by an intent with a "lower" priority
// 						// keep going to add as a delete path
// 					case priority == keyCacheUpdate[0].Priority():
// 						// list item held by an intent with an equal priority
// 						// check owner
// 						if keyCacheUpdate[0].Owner() != owner {
// 							// not same owner go to next key
// 							continue // go to next key
// 						}
// 						// keep going to add as a delete path
// 					case priority > keyCacheUpdate[0].Priority():
// 						// list item held by an intent with an "higher" priority
// 						continue // go to next key
// 					}
// 				}
// 				add := &sdcpb.Path{
// 					Elem: keyPath.GetElem()[:len(keyPath.Elem)-1],
// 				}
// 				xpAdd := utils.ToXPath(add, false)
// 				if _, ok := added[xpAdd]; ok {
// 					continue // path already added as delete, go to next key
// 				}
// 				log.Debugf("ds=%s | %s | adding delete %s", d.Name(), owner, add)
// 				added[xpAdd] = struct{}{}
// 				deletePaths = append(deletePaths, add)
// 			}
// 		}
// 		// fmt.Println()
// 	}
// 	return deletePaths
// }
