package target

import (
	"context"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/ctree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/gnmi/proto/gnmi"
	gtarget "github.com/openconfig/gnmic/target"
	"github.com/openconfig/gnmic/types"

	log "github.com/sirupsen/logrus"
)

type gnmiTarget struct {
	target *gtarget.Target
	main   *ctree.Tree
}

func newGNMITarget(ctx context.Context, name string, cfg *config.SBI, main *ctree.Tree) (*gnmiTarget, error) {
	gt := &gnmiTarget{main: main}
	tc := &types.TargetConfig{
		Name:    name,
		Address: cfg.Address,
		Timeout: 10 * time.Second,
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
	gt.target = gtarget.NewTarget(tc)
	err := gt.target.CreateGNMIClient(ctx)
	if err != nil {
		return nil, err
	}

	return gt, nil
}

func (t *gnmiTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}

func (t *gnmiTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	setReq := &gnmi.SetRequest{
		Delete:  make([]*gnmi.Path, 0, len(req.GetDelete())),
		Replace: make([]*gnmi.Update, 0, len(req.GetReplace())),
		Update:  make([]*gnmi.Update, 0, len(req.GetUpdate())),
	}
	for _, del := range req.GetDelete() {
		setReq.Delete = append(setReq.Delete, toGNMIPath(del))
	}
	for _, repl := range req.GetReplace() {
		setReq.Replace = append(setReq.Replace, &gnmi.Update{
			Path: toGNMIPath(repl.GetPath()),
			Val:  toGNMITypedValue(repl.GetValue()),
		})
	}
	for _, upd := range req.GetUpdate() {
		setReq.Update = append(setReq.Update, &gnmi.Update{
			Path: toGNMIPath(upd.GetPath()),
			Val:  toGNMITypedValue(upd.GetValue()),
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
			Path: fromGNMIPath(updr.GetPath()),
			Op:   schemapb.UpdateResult_Operation(updr.GetOp()),
		})
	}
	return schemaSetRsp, nil
}

func (t *gnmiTarget) Subscribe() {}

func (t *gnmiTarget) Sync(ctx context.Context) {
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
	defer t.target.StopSubscription("sync")
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
				err := t.main.AddGNMIUpdate(rsp.Update)
				if err != nil {
					log.Errorf("failed to insert gNMI update into main DS: %v", err)
					log.Errorf("failed to insert gNMI update into main DS: %v", rsp.Update)
					goto START
				}
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

func toGNMIPath(p *schemapb.Path) *gnmi.Path {
	if p == nil {
		return nil
	}
	r := &gnmi.Path{
		Origin: p.GetOrigin(),
		Elem:   make([]*gnmi.PathElem, 0, len(p.GetElem())),
		Target: p.GetTarget(),
	}
	for _, pe := range p.GetElem() {
		r.Elem = append(r.Elem, &gnmi.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	return r
}
func fromGNMIPath(p *gnmi.Path) *schemapb.Path {
	if p == nil {
		return nil
	}
	r := &schemapb.Path{
		Origin: p.GetOrigin(),
		Elem:   make([]*schemapb.PathElem, 0, len(p.GetElem())),
		Target: p.GetTarget(),
	}
	for _, pe := range p.GetElem() {
		r.Elem = append(r.Elem, &schemapb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	return r
}

func toGNMITypedValue(v *schemapb.TypedValue) *gnmi.TypedValue {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *schemapb.TypedValue_AnyVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
		}
	case *schemapb.TypedValue_AsciiVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
		}
	case *schemapb.TypedValue_BoolVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
		}
	case *schemapb.TypedValue_BytesVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
		}
	// case *schemapb.TypedValue_DecimalVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
	// 	}
	// case *schemapb.TypedValue_FloatVal:
	// 	return &gnmi.TypedValue{
	// 		Value: &gnmi.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
	// 	}
	case *schemapb.TypedValue_IntVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_IntVal{IntVal: v.GetIntVal()},
		}
	case *schemapb.TypedValue_JsonIetfVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
		}
	case *schemapb.TypedValue_JsonVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
		}
	case *schemapb.TypedValue_LeaflistVal:
		gnmilf := &gnmi.ScalarArray{
			Element: make([]*gnmi.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
		}
		for _, e := range v.GetLeaflistVal().GetElement() {
			gnmilf.Element = append(gnmilf.Element, toGNMITypedValue(e))
		}
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_LeaflistVal{LeaflistVal: gnmilf},
		}
	case *schemapb.TypedValue_ProtoBytes:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
		}
	case *schemapb.TypedValue_StringVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetStringVal()},
		}
	case *schemapb.TypedValue_UintVal:
		return &gnmi.TypedValue{
			Value: &gnmi.TypedValue_UintVal{UintVal: v.GetUintVal()},
		}
	}
	return nil
}

func fromGNMITypedValue(v *gnmi.TypedValue) *schemapb.TypedValue {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *gnmi.TypedValue_AnyVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
		}
	case *gnmi.TypedValue_AsciiVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
		}
	case *gnmi.TypedValue_BoolVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
		}
	case *gnmi.TypedValue_BytesVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
		}
	// case *schemapb.TypedValue_DecimalVal:
	// 	return &schemapb.TypedValue{
	// 		Value: &schemapb.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
	// 	}
	// case *schemapb.TypedValue_FloatVal:
	// 	return &schemapb.TypedValue{
	// 		Value: &schemapb.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
	// 	}
	case *gnmi.TypedValue_IntVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_IntVal{IntVal: v.GetIntVal()},
		}
	case *gnmi.TypedValue_JsonIetfVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
		}
	case *gnmi.TypedValue_JsonVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
		}
	case *gnmi.TypedValue_LeaflistVal:
		schemalf := &schemapb.ScalarArray{
			Element: make([]*schemapb.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
		}
		for _, e := range v.GetLeaflistVal().GetElement() {
			schemalf.Element = append(schemalf.Element, fromGNMITypedValue(e))
		}
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_LeaflistVal{LeaflistVal: schemalf},
		}
	case *gnmi.TypedValue_ProtoBytes:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
		}
	case *gnmi.TypedValue_StringVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_StringVal{StringVal: v.GetStringVal()},
		}
	case *gnmi.TypedValue_UintVal:
		return &schemapb.TypedValue{
			Value: &schemapb.TypedValue_UintVal{UintVal: v.GetUintVal()},
		}
	}
	return nil
}
