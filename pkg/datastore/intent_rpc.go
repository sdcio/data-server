package datastore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/iptecharch/cache/proto/cachepb"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/iptecharch/data-server/pkg/cache"
	"github.com/iptecharch/data-server/pkg/utils"
)

var rawIntentPrefix = "__raw_intent_"

func (d *Datastore) GetIntent(ctx context.Context, req *sdcpb.GetIntentRequest) (*sdcpb.GetIntentResponse, error) {
	r, err := d.getRawIntent(ctx, req.GetIntent())
	if err != nil {
		return nil, err
	}
	rsp := &sdcpb.GetIntentResponse{
		Notification: []*sdcpb.Notification{
			{
				Update: r.GetUpdate(),
			},
		},
	}
	return rsp, nil
}

func (d *Datastore) getIntentFlat(ctx context.Context, intentName string) ([]*sdcpb.Notification, error) {
	notifications := make([]*sdcpb.Notification, 0)
	upds := d.cacheClient.Read(ctx, d.config.Name, &cache.Opts{
		Store: cachepb.Store_INTENDED,
		Owner: intentName,
		// Priority: -1, // TODO: related to next TODO in line 47 (skip owner)
	}, [][]string{{"*"}}, 0)

	for _, upd := range upds {
		if upd.Owner() != intentName {
			continue // TODO: DIRTY temp(?) workaround for 2 intents with the same priority
		}
		scp, err := d.toPath(ctx, upd.GetPath())
		if err != nil {
			return nil, err
		}
		tv, err := upd.Value()
		if err != nil {
			return nil, err
		}
		n := &sdcpb.Notification{
			Timestamp: time.Now().UnixNano(),
			Update: []*sdcpb.Update{{
				Path:  scp,
				Value: tv,
			}},
		}
		notifications = append(notifications, n)
	}
	log.Debugf("intent %s current notifications: %v", intentName, notifications)
	return notifications, nil
}

func (d *Datastore) SetIntent(ctx context.Context, req *sdcpb.SetIntentRequest) (*sdcpb.SetIntentResponse, error) {
	now := time.Now().UnixNano()
	candidateName := fmt.Sprintf("%s-%d", req.GetIntent(), now)
	err := d.CreateCandidate(ctx, &sdcpb.DataStore{
		Type:     sdcpb.Type_CANDIDATE,
		Name:     candidateName,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		// delete candidate
		err := d.cacheClient.DeleteCandidate(ctx, d.Name(), candidateName)
		if err != nil {
			log.Errorf("%s: failed to delete candidate %s: %v", d.Name(), candidateName, err)
		}
	}()
	switch {
	case len(req.GetUpdate()) > 0:
		err = d.SetIntentUpdate(ctx, req, candidateName)
	case req.GetDelete():
		err = d.SetIntentDelete(ctx, req, candidateName)
	}
	if err != nil {
		return nil, err
	}
	return &sdcpb.SetIntentResponse{}, nil
}

func (d *Datastore) SetIntentUpdate(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) error {
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

	// expand intent update values
	updates := make([]*sdcpb.Update, 0, len(req.GetUpdate()))
	for _, upd := range req.GetUpdate() {
		expUpds, err := d.expandUpdate(ctx, upd)
		if err != nil {
			return err
		}
		updates = append(updates, expUpds...)
	}

	// debug start
	if log.GetLevel() >= log.DebugLevel {
		for i, upd := range updates {
			log.Debugf("ds=%s | %s | set intent expanded update.%d: %s", d.Name(), req.GetIntent(), i, upd)
		}
	}
	// debug stop

	// go through all updates from the intent to figure out if they need to be applied
	// based on priority
	for _, upd := range updates {
		cp, err := utils.CompletePath(nil, upd.GetPath())
		if err != nil {
			return err
		}
		// get the current highest priority value for this path
		currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name, &cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, [][]string{cp}, 0)

		switch len(currentCacheEntries) {
		case 0:
			// does not exist, apply new intent update
			setDataReq.Update = append(setDataReq.Update, upd)
			// add delete to remove previous value, TODO: not needed ?
			setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
		case 1:
			switch {
			case currentCacheEntries[0].Priority() < req.GetPriority():
			case currentCacheEntries[0].Priority() == req.GetPriority():
				// exists with same priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
			case currentCacheEntries[0].Priority() > req.GetPriority():
				// exists with a "lower" priority, apply current
				setDataReq.Update = append(setDataReq.Update, upd)
				// add delete to remove previous value
				setDataReq.Delete = append(setDataReq.Delete, upd.GetPath())
			}
		default:
			panic(currentCacheEntries)
		}
	}
	//
	// get current intent notifications
	intentNotifications, err := d.getIntentFlat(ctx, req.GetIntent())
	if err != nil {
		return err
	}

	// build deletes from the current intent
	// // collect applied paths
	appliedPaths := make([]*sdcpb.Path, 0, len(intentNotifications))
	for _, n := range intentNotifications {
		for _, upd := range n.GetUpdate() {
			appliedPaths = append(appliedPaths, upd.GetPath())
		}
	}
	for _, p := range appliedPaths {
		cp, err := utils.CompletePath(nil, p)
		if err != nil {
			return err
		}
		// get the current highest priority value for this path
		currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name, &cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, [][]string{cp}, 0)
		switch len(currentCacheEntries) {
		case 0:
		case 1:
			switch {
			case currentCacheEntries[0].Priority() < req.GetPriority(): // exist with a "higher" priority, do not delete
			case currentCacheEntries[0].Priority() == req.GetPriority():
				// same priority, add it to deletes
				setDataReq.Delete = append(setDataReq.Delete, p)
			case currentCacheEntries[0].Priority() > req.GetPriority():
				setDataReq.Delete = append(setDataReq.Delete, p)
			}
		default:
			panic(currentCacheEntries)
		}
	}
	setDataReq.Delete = d.buildDeletePaths(setDataReq.Delete)
	_, err = d.Set(ctx, setDataReq)
	if err != nil {
		return err
	}
	// apply intent
	err = d.applyIntent(ctx, candidateName, setDataReq)
	if err != nil {
		return err
	}
	// update intent in intended store
	dels := make([][]string, 0)
	update := make([]*cache.Update, 0)
	for _, dp := range intentNotifications {
		for _, upd := range dp.GetUpdate() {
			dcp, err := utils.CompletePath(nil, upd.GetPath())
			if err != nil {
				return err
			}
			dels = append(dels, dcp)
		}
	}
	for _, upd := range updates {
		cup, err := d.cacheClient.NewUpdate(upd)
		if err != nil {
			return err
		}
		update = append(update, cup)
	}
	err = d.cacheClient.Modify(ctx, d.Name(), &cache.Opts{
		Store:    cachepb.Store_INTENDED,
		Owner:    req.GetIntent(),
		Priority: req.GetPriority(),
	}, dels, update)
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

func (d *Datastore) SetIntentDelete(ctx context.Context, req *sdcpb.SetIntentRequest, candidateName string) error {
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
			log.Debugf("intent=%s has applied update.%d: %v", req.GetIntent(), i, upd)
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

		switch len(currentCacheEntries) {
		case 0:
			// should not happen
			// panic(currentCacheEntries)
		case 1:
			if currentCacheEntries[0].Owner() == req.GetIntent() {
				setDataReq.Delete = append(setDataReq.Delete, appliedPaths[idx])
			}
		default:
			// should not happen
			// panic(currentCacheEntries)
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
	setDataReq.Delete = d.buildDeletePaths(setDataReq.Delete)

	// go through the applied paths again and get the highest priority (path,value) after deletion
	for _, cp := range appliedCompletePaths {
		currentCacheEntries := d.cacheClient.Read(ctx, d.Config().Name, &cache.Opts{
			Store: cachepb.Store_INTENDED,
		}, [][]string{cp}, 0)
		switch len(currentCacheEntries) {
		case 0:
			// no next highest
		case 1:
			// there is a next highest
			// fmt.Println("there is a next highest: ", currentCacheEntries[0])
			scp, err := d.toPath(ctx, currentCacheEntries[0].GetPath())
			if err != nil {
				return err
			}
			val, err := currentCacheEntries[0].Value()
			if err != nil {
				return err
			}
			upd := &sdcpb.Update{
				Path:  scp,
				Value: val,
			}
			setDataReq.Update = append(setDataReq.Update, upd)
		default:
			// should not happen
			// panic(currentCacheEntries)
		}
	}

	// write to candidate
	_, err = d.Set(ctx, setDataReq)
	if err != nil {
		return err
	}

	// apply intent
	err = d.applyIntent(ctx, candidateName, setDataReq)
	if err != nil {
		return err
	}
	failed = false
	// delete raw intent
	return d.deleteRawIntent(ctx, req.GetIntent())
}

func (d *Datastore) buildDeletePaths(appliedPaths []*sdcpb.Path) []*sdcpb.Path {
	// sort paths by length
	sort.Slice(appliedPaths, func(i, j int) bool {
		return len(appliedPaths[i].GetElem()) < len(appliedPaths[j].GetElem())
	})

	// this map keeps track of paths added as deletes
	added := make(map[string]struct{})

	deletePaths := make([]*sdcpb.Path, 0, len(appliedPaths))
	for _, p := range appliedPaths {
		for i, pe := range p.GetElem() {
			if pe.GetKey() != nil {
				add := &sdcpb.Path{Elem: p.GetElem()[:i+1]}
				xpAdd := utils.ToXPath(add, false)
				if _, ok := added[xpAdd]; ok {
					continue
				}
				added[xpAdd] = struct{}{}
				deletePaths = append(deletePaths, add)
			}
		}
		xpAdd := utils.ToXPath(p, false)
		if _, ok := added[xpAdd]; ok {
			continue
		}
		added[xpAdd] = struct{}{}
		deletePaths = append(deletePaths, p)
	}

	return deletePaths
}

func (d *Datastore) applyIntent(ctx context.Context, candidateName string, sdreq *sdcpb.SetDataRequest) error {
	// debug start
	// for _, n := range sdreq.GetDelete() {
	// 	fmt.Printf("set delete: %v\n", n)
	// }
	// for _, n := range sdreq.GetUpdate() {
	// 	fmt.Printf("set update: %v\n", n)
	// }
	// debug end
	if candidateName == "" {
		return fmt.Errorf("missing candidate name")
	}
	var err error
	sbiSet := &sdcpb.SetDataRequest{
		Update: []*sdcpb.Update{},
		Delete: []*sdcpb.Path{},
	}

	newDeletePaths := make([]*sdcpb.Path, 0, len(sdreq.GetDelete()))
	newDeletePaths = append(newDeletePaths, sdreq.GetDelete()...)
	sdreq.Delete = d.buildDeletePaths(newDeletePaths)
	log.Debugf("%s:%s notification:\n%s", d.Name(), candidateName, prototext.Format(sdreq))
	// TODO: consider if leafref validation
	// needs to run before must statements validation

	// validate MUST statements
	for _, upd := range sdreq.GetUpdate() {
		log.Debugf("%s:%s validating must statement on path: %v", d.Name(), candidateName, upd.GetPath())
		_, err = d.validateMustStatement(ctx, candidateName, upd.GetPath())
		if err != nil {
			return err
		}
	}

	for _, upd := range sdreq.GetUpdate() {
		log.Debugf("%s:%s validating leafRef on update: %v", d.Name(), candidateName, upd)
		err = d.validateLeafRef(ctx, upd, candidateName)
		if err != nil {
			return err
		}
	}
	// push updates to sbi
	sbiSet = &sdcpb.SetDataRequest{
		Update: sdreq.GetUpdate(),
		Delete: sdreq.GetDelete(),
	}
	log.Debugf("datastore %s/%s applyIntent:\n%s", d.config.Name, candidateName, prototext.Format(sbiSet))

	log.Infof("datastore %s/%s applyIntent: sending a setDataRequest with num_updates=%d, num_replaces=%d, num_deletes=%d",
		d.config.Name, candidateName, len(sbiSet.GetUpdate()), len(sbiSet.GetReplace()), len(sbiSet.GetDelete()))

	// send set request only if there are updates and/or deletes
	if len(sbiSet.GetUpdate())+len(sbiSet.GetReplace())+len(sbiSet.GetDelete()) > 0 {
		rsp, err := d.sbi.Set(ctx, sbiSet)
		if err != nil {
			return err
		}
		log.Debugf("datastore %s/%s SetResponse from SBI: %v", d.config.Name, candidateName, rsp)
	}

	return nil
}

func (d *Datastore) saveRawIntent(ctx context.Context, intentName string, req *sdcpb.SetIntentRequest) error {
	b, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	//
	upd, err := d.cacheClient.NewUpdate(
		&sdcpb.Update{
			Path: &sdcpb.Path{
				Elem: []*sdcpb.PathElem{{Name: rawIntentPrefix + intentName}},
			},
			Value: &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_BytesVal{BytesVal: b},
			},
		},
	)
	if err != nil {
		return err
	}
	err = d.cacheClient.Modify(ctx, d.config.Name,
		&cache.Opts{
			Store: cachepb.Store_METADATA,
		},
		nil,
		[]*cache.Update{upd})
	if err != nil {
		return err
	}
	return nil
}

func (d *Datastore) getRawIntent(ctx context.Context, intentName string) (*sdcpb.SetIntentRequest, error) {
	upds := d.cacheClient.Read(ctx, d.config.Name, &cache.Opts{
		Store: cachepb.Store_METADATA,
	}, [][]string{{rawIntentPrefix + intentName}}, 0)
	if len(upds) == 0 {
		return nil, errors.New("not found")
	}

	val, err := upds[0].Value()
	if err != nil {
		return nil, err
	}
	req := &sdcpb.SetIntentRequest{}
	err = proto.Unmarshal(val.GetBytesVal(), req)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (d *Datastore) deleteRawIntent(ctx context.Context, intentName string) error {
	return d.cacheClient.Modify(ctx, d.config.Name,
		&cache.Opts{
			Store: cachepb.Store_METADATA,
		},
		[][]string{{rawIntentPrefix + intentName}},
		nil)
}
