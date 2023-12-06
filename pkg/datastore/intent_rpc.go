package datastore

import (
	"context"
	"errors"
	"fmt"
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
	log.Debug()
	log.Debugf("ds=%s | %s | current notifications: %v", d.Name(), intentName, notifications)
	log.Debug()
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

func (d *Datastore) applyIntent(ctx context.Context, candidateName string, priority int32, sdreq *sdcpb.SetDataRequest) error {
	if candidateName == "" {
		return fmt.Errorf("missing candidate name")
	}
	log.Debugf("%s: applying intent from candidate %s", d.Name(), sdreq.GetDatastore())

	var err error
	sbiSet := &sdcpb.SetDataRequest{
		Update: []*sdcpb.Update{},
		Delete: []*sdcpb.Path{},
	}

	// newDeletePaths := make([]*sdcpb.Path, 0, len(sdreq.GetDelete()))
	// newDeletePaths = append(newDeletePaths, sdreq.GetDelete()...)
	// sdreq.Delete = d.buildDeletePaths(ctx, priority, newDeletePaths)
	log.Debugf("%s: %s notification:\n%s", d.Name(), candidateName, prototext.Format(sdreq))
	// TODO: consider if leafref validation
	// needs to run before must statements validation

	log.Debugf("%s: validating must statements candidate %s", d.Name(), sdreq.GetDatastore())
	// validate MUST statements
	for _, upd := range sdreq.GetUpdate() {
		log.Debugf("%s: %s validating must statement on path: %v", d.Name(), candidateName, upd.GetPath())
		_, err = d.validateMustStatement(ctx, candidateName, upd.GetPath())
		if err != nil {
			return err
		}
	}

	for _, upd := range sdreq.GetUpdate() {
		log.Debugf("%s: %s validating leafRef on update: %v", d.Name(), candidateName, upd)
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

	log.Debugf("datastore %s/%s applyIntent: sending a setDataRequest with num_updates=%d, num_replaces=%d, num_deletes=%d",
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

func (d *Datastore) updatesAddKeysAsLeaves(updates []*sdcpb.Update) []*sdcpb.Update {
	added := make(map[string]struct{})
	upds := make([]*sdcpb.Update, 0, len(updates))
	for _, upd := range updates {
		upds = append(upds, upd)
		for idx, pe := range upd.GetPath().GetElem() {
			if len(pe.GetKey()) == 0 {
				continue
			}
			//fmt.Printf("u | PE %v has keys\n", pe)
			for k, v := range pe.GetKey() {
				//fmt.Printf("u | PE %v has key %s\n", pe, k)
				p := &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, idx+1),
				}
				for i := 0; i < idx+1; i++ {
					p.Elem[i] = &sdcpb.PathElem{
						Name: upd.GetPath().GetElem()[i].GetName(),
						Key:  copyMap(upd.GetPath().GetElem()[i].GetKey()),
					}
				}
				p.Elem = append(p.Elem, &sdcpb.PathElem{Name: k})
				//fmt.Printf("u | KEY Path: %v\n", p)
				nupd := &sdcpb.Update{
					Path: p,
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{StringVal: v},
					},
				}
				uniqueID := utils.ToXPath(p, false) + ":::" + v
				if _, ok := added[uniqueID]; !ok {
					//fmt.Printf("u | ADDING KEY Path: %v\n", p)
					added[uniqueID] = struct{}{}
					upds = append(upds, nupd)
				}
			}
		}
		//// fmt.Println()
	}
	return upds
}

func (d *Datastore) pathsAddKeysAsLeaves(paths []*sdcpb.Path) []*sdcpb.Path {
	added := make(map[string]struct{})
	npaths := make([]*sdcpb.Path, 0, len(paths))
	for _, p := range paths {
		npaths = append(npaths, p)

		for idx, pe := range p.GetElem() {
			if len(pe.GetKey()) == 0 {
				continue
			}
			for k, v := range pe.GetKey() {
				pp := &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, idx+1),
				}
				for i := 0; i < idx+1; i++ {
					pp.Elem[i] = &sdcpb.PathElem{
						Name: p.GetElem()[i].GetName(),
						Key:  copyMap(p.GetElem()[i].GetKey()),
					}
				}
				pp.Elem = append(pp.Elem, &sdcpb.PathElem{Name: k})

				uniqueID := utils.ToXPath(pp, false) + ":::" + v
				if _, ok := added[uniqueID]; !ok {
					added[uniqueID] = struct{}{}
					npaths = append(npaths, pp)
				}
			}
		}
		// fmt.Println()
	}
	return npaths
}

func (d *Datastore) buildPathsWithKeysAsLeaves(paths []*sdcpb.Path) []*sdcpb.Path {
	added := make(map[string]struct{})
	npaths := make([]*sdcpb.Path, 0, len(paths))
	for _, p := range paths {
		for idx, pe := range p.GetElem() {
			if len(pe.GetKey()) == 0 {
				continue
			}
			for k, v := range pe.GetKey() {
				pp := &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, idx+1),
				}
				for i := 0; i < idx+1; i++ {
					pp.Elem[i] = &sdcpb.PathElem{
						Name: p.GetElem()[i].GetName(),
						Key:  copyMap(p.GetElem()[i].GetKey()),
					}
				}
				pp.Elem = append(pp.Elem, &sdcpb.PathElem{Name: k})

				uniqueID := utils.ToXPath(pp, false) + ":::" + v
				if _, ok := added[uniqueID]; !ok {
					// fmt.Printf("d | ADDING KEY Path: %v\n", pp)
					added[uniqueID] = struct{}{}
					npaths = append(npaths, pp)
				}
			}
		}
		// fmt.Println()
	}
	return npaths
}

func (d *Datastore) cacheUpdateToUpdate(ctx context.Context, cupd *cache.Update) (*sdcpb.Update, error) {
	scp, err := d.toPath(ctx, cupd.GetPath())
	if err != nil {
		return nil, err
	}
	val, err := cupd.Value()
	if err != nil {
		return nil, err
	}
	return &sdcpb.Update{
		Path:  scp,
		Value: val,
	}, nil
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	nm := make(map[string]string, len(m))
	for k, v := range m {
		nm[k] = v
	}
	return nm
}
