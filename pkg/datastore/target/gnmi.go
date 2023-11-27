package target

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/openconfig/gnmi/proto/gnmi"
	gapi "github.com/openconfig/gnmic/pkg/api"
	gtarget "github.com/openconfig/gnmic/pkg/target"
	"github.com/openconfig/gnmic/pkg/types"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/iptecharch/data-server/pkg/config"
	"github.com/iptecharch/data-server/pkg/utils"
)

const (
	syncRetryWaitTime = 10 * time.Second
)

type gnmiTarget struct {
	target *gtarget.Target
}

func newGNMITarget(ctx context.Context, name string, cfg *config.SBI, opts ...grpc.DialOption) (*gnmiTarget, error) {
	tc := &types.TargetConfig{
		Name:       name,
		Address:    cfg.Address,
		Timeout:    10 * time.Second,
		RetryTimer: 2 * time.Second,
		BufferSize: 100,
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
	}
	err := gt.target.CreateGNMIClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	// go gt.Sync(ctx, syncCh)
	return gt, nil
}

func (t *gnmiTarget) Get(ctx context.Context, req *sdcpb.GetDataRequest) (*sdcpb.GetDataResponse, error) {
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
	schemaRsp := &sdcpb.GetDataResponse{
		Notification: make([]*sdcpb.Notification, 0, len(gnmiRsp.GetNotification())),
	}
	for _, n := range gnmiRsp.GetNotification() {
		sn := &sdcpb.Notification{
			Timestamp: n.GetTimestamp(),
			Update:    make([]*sdcpb.Update, 0, len(n.GetUpdate())),
			Delete:    make([]*sdcpb.Path, 0, len(n.GetDelete())),
		}
		for _, upd := range n.GetUpdate() {
			sn.Update = append(sn.Update, &sdcpb.Update{
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

func (t *gnmiTarget) Set(ctx context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {
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
	schemaSetRsp := &sdcpb.SetDataResponse{
		Response:  make([]*sdcpb.UpdateResult, 0, len(rsp.GetResponse())),
		Timestamp: rsp.GetTimestamp(),
	}
	for _, updr := range rsp.GetResponse() {
		schemaSetRsp.Response = append(schemaSetRsp.Response, &sdcpb.UpdateResult{
			Path: utils.FromGNMIPath(rsp.GetPrefix(), updr.GetPath()),
			Op:   sdcpb.UpdateResult_Operation(updr.GetOp()),
		})
	}
	return schemaSetRsp, nil
}

func (t *gnmiTarget) Subscribe() {}

func (t *gnmiTarget) Sync(octx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	if t != nil && t.target != nil && t.target.Config != nil {
		log.Infof("starting target %s sync", t.target.Config.Name)
		log.Infof("sync config: %+v", syncConfig)
	}
	var cancel context.CancelFunc
	var ctx context.Context
	var err error
START:
	if cancel != nil {
		cancel()
	}
	ctx, cancel = context.WithCancel(octx)
	defer cancel()
	for _, gnmiSync := range syncConfig.GNMI {
		switch gnmiSync.Mode {
		case "once":
			err = t.periodicSync(ctx, gnmiSync)
		default:
			err = t.streamSync(ctx, gnmiSync)
		}
		if err != nil {
			log.Errorf("target=%s: failed to sync: %v", t.target.Config.Name, err)
			time.Sleep(syncRetryWaitTime)
			goto START
		}
	}
	defer t.target.StopSubscriptions()

	rspch, errCh := t.target.ReadSubscriptions()
	for {
		select {
		case <-ctx.Done():
			log.Infof("target %s sync stopped: %v", t.target.Config.Name, ctx.Err())
			return
		case rsp := <-rspch:
			switch r := rsp.Response.Response.(type) {
			case *gnmi.SubscribeResponse_Update:
				syncCh <- &SyncUpdate{
					Tree:   rsp.SubscriptionName,
					Update: utils.ToSchemaNotification(r.Update),
				}
			}
		case err := <-errCh:
			if err.Err != nil {
				t.target.StopSubscriptions()
				log.Errorf("%s: sync subscription failed: %v", t.target.Config.Name, err)
				time.Sleep(time.Second)
				goto START
			}
		}
	}
}

func (t *gnmiTarget) Close() {
	t.target.Close()
}

func encoding(e string) int {
	enc, ok := gnmi.Encoding_value[strings.ToUpper(e)]
	if ok {
		return int(enc)
	}
	en, err := strconv.Atoi(e)
	if err != nil {
		return 0
	}
	return en
}

func (t *gnmiTarget) periodicSync(ctx context.Context, gnmiSync *config.GNMISync) error {
	opts := make([]gapi.GNMIOption, 0)
	subscriptionOpts := make([]gapi.GNMIOption, 0)
	for _, p := range gnmiSync.Paths {
		subscriptionOpts = append(subscriptionOpts, gapi.Path(p))
	}
	opts = append(opts,
		gapi.EncodingCustom(encoding(gnmiSync.Encoding)),
		gapi.SubscriptionListModeONCE(),
		gapi.Subscription(subscriptionOpts...),
	)
	subReq, err := gapi.NewSubscribeRequest(opts...)
	if err != nil {
		return err
	}
	// initial subscribe ONCE
	go t.target.Subscribe(ctx, subReq, gnmiSync.Name)
	// periodic subscribe ONCE
	go func(gnmiSync *config.GNMISync) {
		ticker := time.NewTicker(gnmiSync.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.target.Subscribe(ctx, subReq, gnmiSync.Name)
			}
		}
	}(gnmiSync)
	return nil
}

func (t *gnmiTarget) streamSync(ctx context.Context, gnmiSync *config.GNMISync) error {
	opts := make([]gapi.GNMIOption, 0)
	subscriptionOpts := make([]gapi.GNMIOption, 0)
	for _, p := range gnmiSync.Paths {
		subscriptionOpts = append(subscriptionOpts, gapi.Path(p))
	}
	switch gnmiSync.Mode {
	case "sample":
		subscriptionOpts = append(subscriptionOpts, gapi.SubscriptionModeSAMPLE())
	case "on-change":
		subscriptionOpts = append(subscriptionOpts, gapi.SubscriptionModeON_CHANGE())
	}

	if gnmiSync.Interval > 0 {
		subscriptionOpts = append(subscriptionOpts, gapi.SampleInterval(gnmiSync.Interval))
	}
	opts = append(opts,
		gapi.EncodingCustom(encoding(gnmiSync.Encoding)),
		gapi.SubscriptionListModeSTREAM(),
		gapi.Subscription(subscriptionOpts...),
	)
	subReq, err := gapi.NewSubscribeRequest(opts...)
	if err != nil {
		return err

	}
	log.Infof("sync %q: subRequest: %v", gnmiSync.Name, subReq)
	go t.target.Subscribe(ctx, subReq, gnmiSync.Name)
	return nil
}
