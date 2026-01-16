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

package utils

import (
	"context"
	"reflect"

	"github.com/openconfig/gnmi/proto/gnmi"
	logf "github.com/sdcio/logger"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func ToSchemaNotification(ctx context.Context, n *gnmi.Notification) *sdcpb.Notification {
	if n == nil {
		return nil
	}
	sn := &sdcpb.Notification{
		Timestamp: n.GetTimestamp(),
		Update:    make([]*sdcpb.Update, 0, len(n.GetUpdate())),
		Delete:    make([]*sdcpb.Path, 0, len(n.GetDelete())),
	}
	for _, del := range n.GetDelete() {
		sn.Delete = append(sn.Delete, FromGNMIPath(n.GetPrefix(), del))
	}
	for idx, upd := range n.GetUpdate() {
		_ = idx
		scUpd := &sdcpb.Update{
			Path:  FromGNMIPath(n.GetPrefix(), upd.GetPath()),
			Value: FromGNMITypedValue(ctx, upd.GetVal()),
		}
		sn.Update = append(sn.Update, scUpd)
	}
	return sn
}

func FromGNMIPath(pre, p *gnmi.Path) *sdcpb.Path {
	if p == nil {
		return nil
	}
	r := &sdcpb.Path{
		Origin:      pre.GetOrigin(),
		Elem:        make([]*sdcpb.PathElem, 0, len(pre.GetElem())+len(p.GetElem())),
		Target:      pre.GetTarget(),
		IsRootBased: true,
	}
	for _, pe := range pre.GetElem() {
		r.Elem = append(r.Elem, &sdcpb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	for _, pe := range p.GetElem() {
		r.Elem = append(r.Elem, &sdcpb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	return r
}

func FromGNMITypedValue(ctx context.Context, v *gnmi.TypedValue) *sdcpb.TypedValue {
	log := logf.FromContext(ctx)
	// log.Tracef("FromGNMITypedValue: %T : %v", v, v)
	if v == nil {
		return nil
	}
	// log.Tracef("FromGNMITypedValue - Value: %T : %v", v.GetValue(), v.GetValue())
	switch v.GetValue().(type) {
	// case *gnmi.TypedValue:
	case *gnmi.TypedValue_AnyVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
		}
	case *gnmi.TypedValue_AsciiVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
		}
	case *gnmi.TypedValue_BoolVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
		}
	case *gnmi.TypedValue_BytesVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
		}
	case *gnmi.TypedValue_IntVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_IntVal{IntVal: v.GetIntVal()},
		}
	case *gnmi.TypedValue_JsonIetfVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
		}
	case *gnmi.TypedValue_JsonVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
		}
	case *gnmi.TypedValue_LeaflistVal:
		schemalf := &sdcpb.ScalarArray{
			Element: make([]*sdcpb.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
		}
		for _, e := range v.GetLeaflistVal().GetElement() {
			schemalf.Element = append(schemalf.Element, FromGNMITypedValue(ctx, e))
		}
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_LeaflistVal{LeaflistVal: schemalf},
		}
	case *gnmi.TypedValue_ProtoBytes:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
		}
	case *gnmi.TypedValue_StringVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_StringVal{StringVal: v.GetStringVal()},
		}
	case *gnmi.TypedValue_UintVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_UintVal{UintVal: v.GetUintVal()},
		}
	case *gnmi.TypedValue_DecimalVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DoubleVal{DoubleVal: v.GetDoubleVal()},
		}
	case *gnmi.TypedValue_FloatVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DoubleVal{DoubleVal: float64(v.GetFloatVal())}, //nolint:staticcheck
		}
	case *gnmi.TypedValue_DoubleVal:
		return &sdcpb.TypedValue{
			Value: &sdcpb.TypedValue_DoubleVal{DoubleVal: v.GetDoubleVal()},
		}
	default:
		log.Error(nil, "unhandled type", "type", reflect.TypeOf(v).String(), "value", v)
		return nil
	}
	// return nil
}

func ToGNMIPath(p *sdcpb.Path) *gnmi.Path {
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

func NotificationsEqual(n1, n2 *sdcpb.Notification) bool {
	if n1 == nil && n2 == nil {
		return true
	}
	if n1 == nil || n2 == nil {
		return false
	}
	if len(n1.GetDelete()) != len(n2.GetDelete()) {
		return false
	}
	for i, dp := range n1.GetDelete() {
		if !dp.PathsEqual(n2.GetDelete()[i]) {
			return false
		}
	}
	if len(n1.GetUpdate()) != len(n2.GetUpdate()) {
		return false
	}
	for i, upd := range n1.GetUpdate() {
		if !upd.GetPath().PathsEqual(n2.GetUpdate()[i].GetPath()) {
			return false
		}
		if !upd.GetValue().Equal(n2.GetUpdate()[i].GetValue()) {
			return false
		}
	}
	return true
}
