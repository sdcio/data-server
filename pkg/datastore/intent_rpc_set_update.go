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

	newUpdates        []*sdcpb.Update
	newPaths          []*sdcpb.Path
	newKeyAsLeafPaths []*sdcpb.Path
	// used to figure out removedPaths
	newTree *ctree.Tree
	//
	currentUpdates        []*sdcpb.Update
	currentPaths          []*sdcpb.Path
	currentKeyAsLeafPaths []*sdcpb.Path
	// currentTree           *ctree.Tree
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

	logger.Debugf("set intent update start")
	defer logger.Debugf("set intent update end")

	ic, err := d.newIntentContext(ctx, req, candidateName)
	if err != nil {
		return err
	}

	// debug start
	logger.Debug()
	for i, upd := range ic.newUpdates {
		logger.Debugf("set intent expanded update.%d: %s", i, upd)
	}
	logger.Debug()
	for i, p := range ic.newPaths {
		logger.Debugf("set intent new path.%d: %s", i, p)
	}
	logger.Debug()
	for i, p := range ic.newKeyAsLeafPaths {
		logger.Debugf("set intent newKeyAsLeaf path.%d: %s", i, p)
	}
	logger.Debug()
	for i, upd := range ic.currentUpdates {
		logger.Debugf("set intent current update.%d: %s", i, upd)
	}
	logger.Debug()
	for i, p := range ic.currentPaths {
		logger.Debugf("set intent current path.%d: %s", i, p)
	}
	logger.Debug()
	for i, p := range ic.currentKeyAsLeafPaths {
		logger.Debugf("set intent currentKeyAsLeaf path.%d: %s", i, p)
	}
	logger.Debug()

	logger.Debugf("has %d removed paths", len(ic.removedPathsMap))
	for rmp := range ic.removedPathsMap {
		logger.Debugf("removed path: %s", rmp)
	}
	// debug stop

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
	ccp := make([][]string, 0, len(ic.newUpdates))
	for _, upd := range ic.newUpdates {
		// build complete path (as []string) from update path
		cp, _ := utils.CompletePath(nil, upd.GetPath())
		ccp = append(ccp, cp)
	}
	allCurrentCacheEntries := d.readNewUpdatesHighestPriority(ctx, ccp)
	// go through all updates from the intent to figure out
	// if they need to be applied based on the intent priority.
	logger.Debugf("reading intent paths to be updated from intended store; looking for the highest priority values")
	for _, upd := range ic.newUpdates {
		// build complete path (as []string) from update path
		cp, err := utils.CompletePath(nil, upd.GetPath())
		if err != nil {
			return err
		}
		// get the current highest priority value(s) for this path
		// currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name,
		// 	&cache.Opts{
		// 		Store: cachepb.Store_INTENDED,
		// 	}, [][]string{cp},
		// 	0)
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
				// there is a current value with higher priority
				// add it to the candidate to allow for proper validation
				upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[0])
				if err != nil {
					return err
				}
				setDataReq.Update = append(setDataReq.Update, upd)
			case currentCacheEntries[0].Priority() == req.GetPriority():
				logger.Debugf("path %v | current intended value has an equal priority to the intent: goes in the setData update and delete", cp)
				// exists with same priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				// setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
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
				upd, err := d.cacheUpdateToUpdate(ctx, currentCacheEntries[lcce-1]) // take last cache update(sorted by ts)
				if err != nil {
					return err
				}
				setDataReq.Update = append(setDataReq.Update, upd)
			case currentCacheEntries[0].Priority() == req.GetPriority():
				logger.Debugf("path %v | current intended value has an equal priority to the intent: goes in the setData update and delete", cp)
				// exists with same priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				// setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
			case currentCacheEntries[0].Priority() > req.GetPriority():
				logger.Debugf("path %v | current intended value has an `lower` priority than the intent: new intent update goes in the setData update and delete", cp)
				// exists with a "lower" priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				// setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
			}
		}
	}

	//
	currentUpdatesHighestPriorities := d.readCurrentUpdatesHighestPriorities(ctx, ic.currentPaths, 2)
	// go though current paths to figure out
	for _, p := range ic.currentPaths {
		logger.Debugf("D | has applied path: %v", p)
		cp, _ := utils.CompletePath(nil, p)
		// get the 2 current highest priorities values for this path
		rsrs := currentUpdatesHighestPriorities[strings.Join(cp, ",")]

		// debug start
		for i, rs := range rsrs {
			for j, r := range rs {
				logger.Debugf("DDDD | %d.%d: %v", i, j, r)
			}
		}
		// debug end
		logger.Debugf("D | got %d sets of highest priorities", len(rsrs))
		switch lrs := len(rsrs); lrs {
		case 0: // there should be no current paths in this case, intent created for the first time
		case 1:
			logger.Debugf("D1 | applied path highest update %v=%v", cp, rsrs[0])
			switch lcce := len(rsrs[0]); lcce {
			case 0:
			case 1:
				logger.Debugf("D | paths %v has 1 entry in the intended store: %v", cp, rsrs[0][0])
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
						setDataReq.Delete = append(setDataReq.Delete, p)
					}
				case rsrs[0][0].Priority() > req.GetPriority():
					logger.Debugf("D | path %v | exists with a `lower` priority; goes in the set data request delete", cp)
					// intent has higher priority, delete
					setDataReq.Delete = append(setDataReq.Delete, p)
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
	for rmcp := range ic.removedPathsMap {
		logger.Debugf("RMP1: %s\n", rmcp)
		rmp, err := d.toPath(ctx, strings.Split(rmcp, ","))
		if err != nil {
			return err
		}
		logger.Debugf("RMP2: %s\n", rmp)
		keyPaths := d.buildPathsWithKeysAsLeaves([]*sdcpb.Path{rmp})
		logger.Debugf("RMP3: keyPaths: %v\n", keyPaths)
		if len(keyPaths) == 0 {
			continue
		}
		// check next intended entry after the current one
		nextUpdate := d.getNextPriority(ctx, req.GetPriority(), req.GetName(), strings.Split(rmcp, ","))
		logger.Debugf("RMP4: next update %v = %v", rmcp, nextUpdate)
		if nextUpdate != nil {
			nu, err := d.cacheUpdateToUpdate(ctx, nextUpdate)
			if err != nil {
				return err
			}
			setDataReq.Update = append(setDataReq.Update, nu)
			continue
		}

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
	_, err = d.Set(ctx, setDataReq)
	if err != nil {
		return err
	}

	// apply intent
	err = d.applyIntent(ctx, candidateName, req.GetPriority(), setDataReq)
	if err != nil {
		return err
	}
	logger.Debug()
	logger.Debug("intent is validated")
	logger.Debug("intent is applied")
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
	ic.newUpdates = d.updatesAddKeysAsLeaves(ic.newUpdates)
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
	return nil
}

// buildRemovedPaths populates the removedPaths field without the keys as leaves.
// it adds explicit deletes for each leaf missing in an update.
func (d *Datastore) buildRemovedPaths(ctx context.Context, ic *intentContext) error {
	var err error
	// build new tree
	for _, nu := range ic.newUpdates {
		err = ic.newTree.AddSchemaUpdate(nu)
		if err != nil {
			return err
		}
	}
	// query current paths from new tree
	// the ones that don't exist are added to removedPaths
	for _, p := range ic.currentPaths {
		cp, _ := utils.CompletePath(nil, p)
		if ic.newTree.GetLeaf(cp) == nil {
			ic.removedPathsMap[strings.Join(cp, ",")] = struct{}{}
			// ic.removedPaths = append(ic.removedPaths, p)
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
		req:               req,
		candidateName:     candidate,
		newUpdates:        []*sdcpb.Update{},
		newPaths:          []*sdcpb.Path{},
		newKeyAsLeafPaths: []*sdcpb.Path{},
		newTree:           &ctree.Tree{},

		currentUpdates:        []*sdcpb.Update{},
		currentPaths:          []*sdcpb.Path{},
		currentKeyAsLeafPaths: []*sdcpb.Path{},
		// currentTree:           &ctree.Tree{},
		// removedPaths:    []*sdcpb.Path{},
		removedPathsMap: map[string]struct{}{},
	}
	// expand intent updates values
	ic.newUpdates, err = d.expandUpdates(ctx, req.GetUpdate())
	if err != nil {
		return nil, err
	}
	// build new paths
	for _, upd := range ic.newUpdates {
		ic.newPaths = append(ic.newPaths, upd.GetPath())
	}
	ic.newKeyAsLeafPaths = extractKeyLeafPaths(ic.newPaths)
	//
	// get current intent notifications
	intentNotifications, err := d.getIntentFlat(ctx, req.GetIntent())
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
	err = d.buildRemovedPaths(ctx, ic)
	if err != nil {
		return nil, err
	}
	return ic, nil
}

func extractKeyLeafPaths(ps []*sdcpb.Path) []*sdcpb.Path {
	if len(ps) == 0 {
		return nil
	}
	//
	rs := make([]*sdcpb.Path, 0, len(ps))
	added := make(map[string]struct{})

	for _, p := range ps {
		for idx, pe := range p.GetElem() {
			if len(pe.GetKey()) == 0 {
				continue
			}
			// path has keys
			for k := range pe.GetKey() {
				keyPath := &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, idx+1),
				}
				for i := 0; i < idx+1; i++ {
					keyPath.Elem[i] = &sdcpb.PathElem{
						Name: p.GetElem()[i].GetName(),
						Key:  copyMap(p.GetElem()[i].GetKey()),
					}
				}
				keyPath.Elem = append(keyPath.Elem, &sdcpb.PathElem{Name: k})
				kxp := utils.ToXPath(keyPath, false)
				if _, ok := added[kxp]; !ok {
					added[kxp] = struct{}{}
					rs = append(rs, keyPath)
				}
			}
		}
	}
	//
	return rs
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

func (d *Datastore) readCurrentUpdatesHighestPriorities(ctx context.Context, sdcps []*sdcpb.Path, count uint64) map[string][][]*cache.Update {
	paths := make([][]string, 0, len(sdcps))
	for _, p := range sdcps {
		cp, _ := utils.CompletePath(nil, p)
		paths = append(paths, cp)
	}

	currentCacheEntries := d.cacheClient.Read(ctx, d.Name(),
		&cache.Opts{
			Store:         cachepb.Store_INTENDED,
			PriorityCount: count,
		}, paths,
		0)
	if len(currentCacheEntries) == 0 {
		return nil
	}
	rs := make(map[string][][]*cache.Update)
	grouppings := make(map[string]map[int32][]*cache.Update)
	for _, cce := range currentCacheEntries {
		sp := strings.Join(cce.GetPath(), ",")
		if _, ok := rs[sp]; !ok {
			rs[sp] = make([][]*cache.Update, 0, 1)
		}
		if _, ok := grouppings[sp]; !ok {
			grouppings[sp] = make(map[int32][]*cache.Update)
		}
		if _, ok := grouppings[sp][cce.Priority()]; !ok {
			grouppings[sp][cce.Priority()] = make([]*cache.Update, 0, 1)
		}
		grouppings[sp][cce.Priority()] = append(grouppings[sp][cce.Priority()], cce)
	}
	for sp, groupping := range grouppings {
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
