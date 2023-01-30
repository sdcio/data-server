package target

import (
	"context"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/openconfig/gnmi/proto/gnmi"
	gtarget "github.com/openconfig/gnmic/target"
	"github.com/openconfig/gnmic/types"

	log "github.com/sirupsen/logrus"
)

type gnmiTarget struct {
	target *gtarget.Target
}

func newGNMITarget(ctx context.Context, name string, cfg *config.SBI) (*gnmiTarget, error) {
	tc := &types.TargetConfig{
		Name:       name,
		Address:    cfg.Address,
		Timeout:    10 * time.Second,
		RetryTimer: 2 * time.Second,
	}
	if cfg.Credentials != nil {
		tc.Username = &cfg.Credentials.Username
		tc.Password = &cfg.Credentials.Password
	}
	if cfg.TLS != nil {
		tc.TLSCA = &cfg.TLS.CA
		tc.TLSCert = &cfg.TLS.Cert
		tc.TLSKey = &cfg.TLS.Key
		tc.SkipVerify = &cfg.TLS.SkipVerify
	} else {
		tc.Insecure = pointer.ToBool(true)
	}
	gt := &gnmiTarget{
		target: gtarget.NewTarget(tc),
		// syncCh: syncCh,
	}
	err := gt.target.CreateGNMIClient(ctx)
	if err != nil {
		return nil, err
	}
	// go gt.Sync(ctx, syncCh)
	return gt, nil
}

func (t *gnmiTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	gnmiReq := &gnmi.GetRequest{
		Path:     make([]*gnmi.Path, 0, len(req.GetPath())),
		Encoding: gnmi.Encoding_ASCII,
	}
	for _, p := range req.GetPath() {
		gnmiReq.Path = append(gnmiReq.Path, utils.ToGNMIPath(p))
	}
	gnmiRsp, err := t.target.Get(ctx, gnmiReq)
	if err != nil {
		return nil, err
	}
	schemaRsp := &schemapb.GetDataResponse{
		Notification: make([]*schemapb.Notification, 0, len(gnmiRsp.GetNotification())),
	}
	for _, n := range gnmiRsp.GetNotification() {
		sn := &schemapb.Notification{
			Timestamp: n.GetTimestamp(),
			Update:    make([]*schemapb.Update, 0, len(n.GetUpdate())),
			Delete:    make([]*schemapb.Path, 0, len(n.GetDelete())),
		}
		for _, upd := range n.GetUpdate() {
			sn.Update = append(sn.Update, &schemapb.Update{
				Path:  utils.FromGNMIPath(n.GetPrefix(), upd.GetPath()),
				Value: utils.FromGNMITypedValue(upd.GetVal()),
			})
		}
		for _, del := range n.GetDelete() {
			sn.Delete = append(sn.Delete, utils.FromGNMIPath(n.GetPrefix(), del))
		}
		schemaRsp.Notification = append(schemaRsp.Notification, sn)
	}
	return schemaRsp, nil
}

func (t *gnmiTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	setReq := &gnmi.SetRequest{
		Delete:  make([]*gnmi.Path, 0, len(req.GetDelete())),
		Replace: make([]*gnmi.Update, 0, len(req.GetReplace())),
		Update:  make([]*gnmi.Update, 0, len(req.GetUpdate())),
	}
	for _, del := range req.GetDelete() {
		setReq.Delete = append(setReq.Delete, utils.ToGNMIPath(del))
	}
	for _, repl := range req.GetReplace() {
		setReq.Replace = append(setReq.Replace, &gnmi.Update{
			Path: utils.ToGNMIPath(repl.GetPath()),
			Val:  utils.ToGNMITypedValue(repl.GetValue()),
		})
	}
	for _, upd := range req.GetUpdate() {
		setReq.Update = append(setReq.Update, &gnmi.Update{
			Path: utils.ToGNMIPath(upd.GetPath()),
			Val:  utils.ToGNMITypedValue(upd.GetValue()),
		})
	}
	rsp, err := t.target.Set(ctx, setReq)
	if err != nil {
		return nil, err
	}
	schemaSetRsp := &schemapb.SetDataResponse{
		Response:  make([]*schemapb.UpdateResult, 0, len(rsp.GetResponse())),
		Timestamp: rsp.GetTimestamp(),
	}
	for _, updr := range rsp.GetResponse() {
		schemaSetRsp.Response = append(schemaSetRsp.Response, &schemapb.UpdateResult{
			Path: utils.FromGNMIPath(rsp.GetPrefix(), updr.GetPath()),
			Op:   schemapb.UpdateResult_Operation(updr.GetOp()),
		})
	}
	return schemaSetRsp, nil
}

func (t *gnmiTarget) Subscribe() {}

func (t *gnmiTarget) Sync(ctx context.Context, syncCh chan *schemapb.Notification) {
	log.Infof("starting target %s sync", t.target.Config.Name)
START:
	go t.target.Subscribe(ctx, &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				// Prefix: &gnmi.Path{},
				Subscription: []*gnmi.Subscription{
					{
						Mode: gnmi.SubscriptionMode_ON_CHANGE,
					},
				},
				Mode:     gnmi.SubscriptionList_STREAM,
				Encoding: gnmi.Encoding_ASCII,
			},
		},
	}, "sync")
	rspch, errCh := t.target.ReadSubscriptions()
	// log.Info("reading target subs")
	for {
		select {
		case <-ctx.Done():
			log.Infof("target %s sync stopped: %v", t.target.Config.Name, ctx.Err())
			return
		case rsp := <-rspch:
			switch rsp := rsp.Response.Response.(type) {
			case *gnmi.SubscribeResponse_Update:
				syncCh <- utils.ToSchemaNotification(rsp.Update)
			}
		case err := <-errCh:
			if err.Err != nil {
				log.Errorf("sync subscription failed: %v", err)
				goto START
			}
		}
	}
}

func (t *gnmiTarget) Close() {
	t.target.Close()
}
