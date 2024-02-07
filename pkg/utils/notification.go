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
	"github.com/openconfig/gnmi/proto/gnmi"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

func ToSchemaNotification(n *gnmi.Notification) *sdcpb.Notification {
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
	for _, upd := range n.GetUpdate() {
		scUpd := &sdcpb.Update{
			Path:  FromGNMIPath(n.GetPrefix(), upd.GetPath()),
			Value: FromGNMITypedValue(upd.GetVal()),
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
		Origin: pre.GetOrigin(),
		Elem:   make([]*sdcpb.PathElem, 0, len(pre.GetElem())+len(p.GetElem())),
		Target: pre.GetTarget(),
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

func FromGNMITypedValue(v *gnmi.TypedValue) *sdcpb.TypedValue {
	log.Tracef("FromGNMITypedValue: %T : %v", v, v)
	if v == nil {
		return nil
	}
	log.Tracef("FromGNMITypedValue - Value: %T : %v", v.GetValue(), v.GetValue())
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
	// case *sdcpb.TypedValue_DecimalVal:
	// 	return &sdcpb.TypedValue{
	// 		Value: &sdcpb.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
	// 	}
	// case *sdcpb.TypedValue_FloatVal:
	// 	return &sdcpb.TypedValue{
	// 		Value: &sdcpb.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
	// 	}
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
			schemalf.Element = append(schemalf.Element, FromGNMITypedValue(e))
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
			Value: &sdcpb.TypedValue_DoubleVal{DoubleVal: float64(v.GetFloatVal())},
		}
	default:
		log.Errorf("FromGNMITypedValue unhandled type: %T: %v", v, v)
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

// func toGNMITypedValue(v *sdcpb.TypedValue) *gnmi.TypedValue {
// 	if v == nil {
// 		return nil
// 	}
// 	switch v.GetValue().(type) {
// 	case *sdcpb.TypedValue_AnyVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
// 		}
// 	case *sdcpb.TypedValue_AsciiVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
// 		}
// 	case *sdcpb.TypedValue_BoolVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
// 		}
// 	case *sdcpb.TypedValue_BytesVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
// 		}
// 	// case *sdcpb.TypedValue_DecimalVal:
// 	// 	return &gnmi.TypedValue{
// 	// 		Value: &gnmi.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
// 	// 	}
// 	// case *sdcpb.TypedValue_FloatVal:
// 	// 	return &gnmi.TypedValue{
// 	// 		Value: &gnmi.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
// 	// 	}
// 	case *sdcpb.TypedValue_IntVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_IntVal{IntVal: v.GetIntVal()},
// 		}
// 	case *sdcpb.TypedValue_JsonIetfVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
// 		}
// 	case *sdcpb.TypedValue_JsonVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
// 		}
// 	case *sdcpb.TypedValue_LeaflistVal:
// 		gnmilf := &gnmi.ScalarArray{
// 			Element: make([]*gnmi.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
// 		}
// 		for _, e := range v.GetLeaflistVal().GetElement() {
// 			gnmilf.Element = append(gnmilf.Element, toGNMITypedValue(e))
// 		}
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_LeaflistVal{LeaflistVal: gnmilf},
// 		}
// 	case *sdcpb.TypedValue_ProtoBytes:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
// 		}
// 	case *sdcpb.TypedValue_StringVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetStringVal()},
// 		}
// 	case *sdcpb.TypedValue_UintVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_UintVal{UintVal: v.GetUintVal()},
// 		}
// 	}
// 	return nil
// }
