package datastore

import (
	"context"
	"sort"
	"strings"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/datastore/ctree"
	"github.com/iptecharch/data-server/pkg/utils"
)

type intentContext struct {
	req           *sdcpb.SetIntentRequest
	candidateName string

	newUpdates       []*sdcpb.Update
	newCompletePaths [][]string
	//
	currentUpdates        []*sdcpb.Update
	currentPaths          []*sdcpb.Path
	currentKeyAsLeafPaths []*sdcpb.Path
	//
	// removedPaths    []*sdcpb.Path
	removedPathsMap map[string]struct{}
}

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

	ic, err := d.newIntentContext(ctx, req, candidateName)
	if err != nil {
		return err
	}

	// TODO: validate updates earlier

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
	//
	allCurrentCacheEntries := d.readNewUpdatesHighestPriority(ctx, ic.newCompletePaths)
	// PH1: go through all updates from the intent to figure out
	// if they need to be applied based on the intent priority.
	logger.Debugf("reading intent paths to be updated from intended store; looking for the highest priority values")

	for i, upd := range ic.newUpdates {
		cp := ic.newCompletePaths[i]
		// get the current highest priority value(s) for this path
		currentCacheEntries := allCurrentCacheEntries[strings.Join(cp, ",")]
		logger.Debugf("highest update (p=updates): %v=%v", cp, currentCacheEntries)
		// check if an update with "higher" priority exists
		switch lcce := len(currentCacheEntries); lcce {
		case 0:
			logger.Debugf("path %v has no entries in the intended store: goes in setData update", cp)
			// does not exist, apply new intent update
			setDataReq.Update = append(setDataReq.Update, upd)
			// add delete to remove previous value, TODO: not needed ? deviation ?
			// setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
		case 1:
			logger.Debugf("path %v has an entry in the intended store: checking priority", cp)
			switch {
			case currentCacheEntries[0].Priority() < req.GetPriority():
				logger.Debugf("path %v | current intended value has a `higher` priority that the intent: current intended value goes in the setData update", cp)
				// there is a current value with higher priority.
				// add it to the candidate to allow for proper validation.
				cupd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
				if err != nil {
					return err
				}
				setDataReq.Update = append(setDataReq.Update, cupd)
			case currentCacheEntries[0].Priority() == req.GetPriority():
				logger.Debugf("path %v | current intended value has an equal priority to the intent: goes in the setData update and delete", cp)
				// exists with same priority

				// if the highest priority is the current intent being applied,
				// set the new received value
				if currentCacheEntries[0].Owner() == req.GetIntent() {
					setDataReq.Update = append(setDataReq.Update, upd)
				} else {
					// else: the highest priority is owned by another intent
					// set the value from cache for validation
					cupd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
					if err != nil {
						return err
					}
					setDataReq.Update = append(setDataReq.Update, cupd)
				}
			case currentCacheEntries[0].Priority() > req.GetPriority():
				logger.Debugf("path %v | current intended value has an `lower` priority than the intent: new intent update goes in the setData update and delete", cp)
				// exists with a "lower" priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
			}
		default:
			logger.Debugf("path %v has %d entries in the intended store: checking priority and owner", cp, lcce)
			for i, cce := range currentCacheEntries {
				logger.Debugf("path %v has entry.%d: %v", cp, i, cce)
			}

			switch {
			case currentCacheEntries[0].Priority() < req.GetPriority():
				logger.Debugf("path %v | current intended value has a `higher` priority that the intent: current intended value goes in the setData update", cp)
				// there is a current value with higher priority
				// add it to the candidate to allow for proper validation
				cupd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
				if err != nil {
					return err
				}
				setDataReq.Update = append(setDataReq.Update, cupd)
			case currentCacheEntries[0].Priority() == req.GetPriority():
				logger.Debugf("path %v | current intended value has an equal priority to the intent: goes in the setData update and delete", cp)
				// if the highest priority is the current intent being applied,
				// set the new received value
				if currentCacheEntries[0].Owner() == req.GetIntent() {
					setDataReq.Update = append(setDataReq.Update, upd)
				} else {
					// else: the highest priority is owned by another intent
					// set the value from cache for validation
					cupd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
					if err != nil {
						return err
					}
					setDataReq.Update = append(setDataReq.Update, cupd)
				}
			case currentCacheEntries[0].Priority() > req.GetPriority():
				logger.Debugf("path %v | current intended value has an `lower` priority than the intent: new intent update goes in the setData update and delete", cp)
				// exists with a "lower" priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
			}
		}
	}

	//
	currentPaths := make([][]string, 0, len(ic.currentPaths))
	for _, p := range ic.currentPaths {
		cp, _ := utils.CompletePath(nil, p)
		currentPaths = append(currentPaths, cp)
	}

	currentUpdatesHighestPriorities := d.readCurrentUpdatesHighestPriorities(ctx, currentPaths, 2)
	// go though current paths to figure out
	for i, cp := range currentPaths {
		logger.Debugf("D | has applied path: %v", cp)
		// cp, _ := utils.CompletePath(nil, p)
		// get the 2 current highest priorities values for this path
		rsrs := currentUpdatesHighestPriorities[strings.Join(cp, ",")]

		// debug start
		for i, rs := range rsrs {
			for j, r := range rs {
				logger.Debugf("DDDD | %d.%d: %v", i, j, r)
			}
		}
		// debug end
		logger.Debugf("D | got %d set(s) of highest priorities", len(rsrs))
		switch lrs := len(rsrs); lrs {
		case 0: // there should be no current paths in this case, intent created for the first time
		case 1:
			logger.Debugf("D1 | applied path highest update %v=%v", cp, rsrs[0])
			switch lcce := len(rsrs[0]); lcce {
			case 0:
			case 1:
				logger.Debugf("D | path %v has 1 entry in the intended store: %v", cp, rsrs[0][0])
				switch {
				// exist with a "higher" priority, do not delete
				case rsrs[0][0].Priority() < req.GetPriority():
					logger.Debugf("D | path %v | exists with a `higher` priority: do nothing", cp)
				case rsrs[0][0].Priority() == req.GetPriority():
					logger.Debugf("D | path %v | exists with an equal priority: check owner", cp)
					// TODO: here? check next priority and apply it if it exists
					if rsrs[0][0].Owner() == req.GetIntent() {
						// same priority and same owner, add it to deletes
						logger.Debugf("D | path %v | exists with an equal priority and same owner: add to set data request delete", cp)
						setDataReq.Delete = append(setDataReq.Delete, ic.currentPaths[i])
					}
				case rsrs[0][0].Priority() > req.GetPriority():
					logger.Debugf("D | path %v | exists with a `lower` priority; goes in the set data request delete", cp)
					// intent has higher priority, delete
					setDataReq.Delete = append(setDataReq.Delete, ic.currentPaths[i])
					// TODO: here? check next priority and apply it if it exists
				}
			default:
				logger.Debugf("D | paths %v has %d entries in the intended store", cp, lcce)
				for i, cce := range rsrs[0] {
					logger.Debugf("D | path %v has entry.%d: %v", cp, i, cce)
				}
				switch {
				// exist with a "higher" priority, do not delete
				case rsrs[0][0].Priority() < req.GetPriority():
					logger.Debugf("D | path %v | exists with a `higher` priority: do nothing", cp)
				case rsrs[0][0].Priority() == req.GetPriority():
					logger.Debugf("D | path %v | exists with an equal priority: check owner", cp)
					// check if the latest update is owned by this intent
					selectedCacheUpdate := rsrs[0][lcce-1]
					if selectedCacheUpdate.Owner() == req.GetIntent() { // this intent owns the latest update
						rmcp := strings.Join(cp, ",")
						if _, ok := ic.removedPathsMap[rmcp]; ok { // this intent is removing this leaf
							selectedCacheUpdate = rsrs[0][lcce-2] // replace with the next update in the intended store
							logger.Debugf("D | path %v | exists with an equal priority and this intent is the latest; pick next cacheUpdate: %v", cp, selectedCacheUpdate)
							delete(ic.removedPathsMap, rmcp) // delete path from removed paths map
							upd, err := d.cacheUpdateToUpdate(ctx, selectedCacheUpdate)
							if err != nil {
								return err
							}
							setDataReq.Update = append(setDataReq.Update, upd)
						}
					}
				case rsrs[0][0].Priority() > req.GetPriority(): // Can this happen ????
					logger.Debugf("D | path %v | !! UNHANDLED !! : exists with a `lower` priority; goes in the set data request delete", cp)
					logger.Debugf("D | path %v | %v", cp, rsrs[0][0])
					// intent has higher priority, delete
					// setDataReq.Delete = append(setDataReq.Delete, p)
				}
			}
		case 2:
			logger.Debugf("D2 | applied path highest update %v=%+v", cp, rsrs)
		}
	}

	// update context with new updates
	// TODO: add removed paths as deletes if they are not present in the setDataReq.Update
	pathsToRemove := make([]*sdcpb.Path, 0, len(ic.removedPathsMap))
	uniqueAdded := map[string]struct{}{}
	// TODO: retrieve all next priorities
	for rmcp := range ic.removedPathsMap {
		logger.Debugf("RMP1: %s", rmcp)
		rmp, err := d.toPath(ctx, strings.Split(rmcp, ","))
		if err != nil {
			return err
		}
		logger.Debugf("RMP2: %s", rmp)
		keyPaths := d.buildPathsWithKeysAsLeaves([]*sdcpb.Path{rmp})
		logger.Debugf("RMP3: keyPaths: %v", keyPaths)
		if len(keyPaths) == 0 {
			continue
		}
		// check next intended entry after the current one
		nextUpdate := d.getNextPriority(ctx, req.GetPriority(), req.GetIntent(), strings.Split(rmcp, ","))
		logger.Debugf("RMP4: next update %v = %v", rmcp, nextUpdate)
		if nextUpdate != nil {
			nu, err := d.cacheUpdateToUpdate(ctx, nextUpdate)
			if err != nil {
				return err
			}
			setDataReq.Update = append(setDataReq.Update, nu)
			continue
		}

		// no next update, check if the removed paths are referenced by any other
		// update. if not delete they keyPaths.

		// ugly start
	NEXT_PATH:
		for _, kp := range keyPaths {
			logger.Debugf("keyPath: %v", kp)
			kpNOKey := proto.Clone(kp).(*sdcpb.Path)
			kpNOKey.Elem = kpNOKey.Elem[:len(kp.Elem)-1]
			logger.Debugf("keyPathNOKey: %v", kpNOKey)
			kpxp := utils.ToXPath(kpNOKey, false)

			for _, upd := range setDataReq.GetUpdate() {
				updxp := utils.ToXPath(upd.GetPath(), false)
				if strings.HasPrefix(updxp, kpxp) {
					continue NEXT_PATH // the removed path is being updated
				}
			}

			if _, ok := uniqueAdded[kpxp]; ok {
				continue NEXT_PATH // the path has been added to the list already
			}
			uniqueAdded[kpxp] = struct{}{}
			// the removed path is not being updated
			pathsToRemove = append(pathsToRemove, kpNOKey)
		}
		// ugly end
	}

	logger.Debugf("got %d paths to remove", len(pathsToRemove))
	for i, ptrm := range pathsToRemove {
		logger.Debugf("path to remove: %d: %v", i, ptrm)
		setDataReq.Delete = append(setDataReq.Delete, ptrm)
	}
	//

	// TODO: cleanup set data request:
	// - remove redundant deletes
	// - group updates into updates with JSON TypedValues?

	logger.Debug()
	logger.Debugf("done building set data request for the candidate")
	logger.Debugf("set data request: START")

	for i, upd := range setDataReq.GetUpdate() {
		logger.Debugf("set data request update.%d: %v", i, upd)
	}
	for i, del := range setDataReq.GetDelete() {
		logger.Debugf("set data request delete.%d: %v", i, del)
	}

	logger.Debugf("set data request: END")
	logger.Debug()
	// fmt.Println(prototext.Format(setDataReq))
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

	//// deletes
	delPaths := make([]*sdcpb.Path, 0)
	for _, upd := range ic.currentUpdates {
		delPaths = append(delPaths, upd.GetPath())
	}

	// add paths ending with keys to deletes,
	// to be used to delete intended notifications
	delPaths = d.pathsAddKeysAsLeaves(delPaths)
	dels := make([][]string, 0, len(delPaths))
	for _, dp := range delPaths {
		dcp, err := utils.CompletePath(nil, dp)
		if err != nil {
			return err
		}
		dels = append(dels, dcp)
	}

	// add paths ending with keys to updates
	cacheUpdates := make([]*cache.Update, 0, len(ic.newUpdates))
	for _, upd := range ic.newUpdates {
		cup, err := d.cacheClient.NewUpdate(upd)
		if err != nil {
			return err
		}
		cacheUpdates = append(cacheUpdates, cup)
	}
	for _, del := range dels {
		logger.Debugf("on intended deleting: %v", del)
	}
	// 	logger.Debugf()
	for _, update := range cacheUpdates {
		logger.Debugf("on intended updating: %v", update)
	}
	// logger.Debugf()
	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, dels, cacheUpdates)
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

// buildRemovedPaths populates the removedPaths field without the keys as leaves.
// it adds explicit deletes for each leaf missing in an update.
func (ic *intentContext) buildRemovedPaths(ctx context.Context) error {
	var err error
	// this tree is used to figure out the
	// paths that don't exist anymore in an intent update.
	t := ctree.Tree{}
	// build new tree
	for _, nu := range ic.newUpdates {
		err = t.AddSchemaUpdate(nu)
		if err != nil {
			return err
		}
	}
	// query current paths from new tree
	// the ones that don't exist are added to removedPaths
	for _, p := range ic.currentPaths {
		cp, _ := utils.CompletePath(nil, p)
		if t.GetLeaf(cp) == nil {
			ic.removedPathsMap[strings.Join(cp, ",")] = struct{}{}
		}
	}
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

func (d *Datastore) newIntentContext(ctx context.Context, req *sdcpb.SetIntentRequest, candidate string) (*intentContext, error) {
	var err error
	ic := &intentContext{
		req:           req,
		candidateName: candidate,

		newUpdates:       []*sdcpb.Update{},
		newCompletePaths: [][]string{},

		currentUpdates:        []*sdcpb.Update{},
		currentPaths:          []*sdcpb.Path{},
		currentKeyAsLeafPaths: []*sdcpb.Path{},

		removedPathsMap: map[string]struct{}{},
	}
	// expand intent updates values
	ic.newUpdates, err = d.expandUpdates(ctx, req.GetUpdate(), true)
	if err != nil {
		return nil, err
	}
	// build new complete paths ([]string)
	ic.newCompletePaths = make([][]string, 0, len(ic.newUpdates))
	for _, upd := range ic.newUpdates {
		// build complete path (as []string) from update path
		cp, _ := utils.CompletePath(nil, upd.GetPath())
		ic.newCompletePaths = append(ic.newCompletePaths, cp)
	}

	// get current intent notifications
	intentNotifications, err := d.getIntentFlatNotifications(ctx, req.GetIntent(), req.GetPriority())
	if err != nil {
		return nil, err
	}
	// init currentUpdates and currentPaths
	for _, n := range intentNotifications {
		for _, upd := range n.GetUpdate() {
			ic.currentUpdates = append(ic.currentUpdates, upd)
			if pathIsKeyAsLeaf(upd.GetPath()) {
				ic.currentKeyAsLeafPaths = append(ic.currentKeyAsLeafPaths, upd.GetPath())
				continue
			}
			ic.currentPaths = append(ic.currentPaths, upd.GetPath())
		}
	}

	err = ic.buildRemovedPaths(ctx)
	if err != nil {
		return nil, err
	}
	log.Infof("intent=%s: %d new updates", req.GetIntent(), len(ic.newUpdates))
	log.Infof("intent=%s: %d new cPaths", req.GetIntent(), len(ic.newCompletePaths))
	log.Infof("intent=%s: %d current updates", req.GetIntent(), len(ic.currentUpdates))
	log.Infof("intent=%s: %d current paths", req.GetIntent(), len(ic.currentPaths))
	log.Infof("intent=%s: %d current paths with key as leaf", req.GetIntent(), len(ic.currentKeyAsLeafPaths))
	log.Infof("intent=%s: %d removed paths", req.GetIntent(), len(ic.removedPathsMap))
	// debug start
	log.Debug()
	for i, upd := range ic.newUpdates {
		log.Debugf("set intent expanded update.%d: %s", i, upd)
	}
	// logger.Debug()
	log.Debug()
	for i, upd := range ic.currentUpdates {
		log.Debugf("set intent current update.%d: %s", i, upd)
	}
	log.Debug()
	for i, p := range ic.currentPaths {
		log.Debugf("set intent current path.%d: %s", i, p)
	}
	log.Debug()
	for i, p := range ic.currentKeyAsLeafPaths {
		log.Debugf("set intent currentKeyAsLeaf path.%d: %s", i, p)
	}
	log.Debug()

	log.Debugf("has %d removed paths", len(ic.removedPathsMap))
	for rmp := range ic.removedPathsMap {
		log.Debugf("removed path: %s", rmp)
	}
	// debug stop
	return ic, nil
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
