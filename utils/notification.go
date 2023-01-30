package utils

import (
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
)

func ToSchemaNotification(n *gnmi.Notification) *schemapb.Notification {
	if n == nil {
		return nil
	}
	sn := &schemapb.Notification{
		Timestamp: n.GetTimestamp(),
		Update:    make([]*schemapb.Update, 0, len(n.GetUpdate())),
		Delete:    make([]*schemapb.Path, 0, len(n.GetDelete())),
	}
	for _, del := range n.GetDelete() {
		sn.Delete = append(sn.Delete, FromGNMIPath(n.GetPrefix(), del))
	}
	for _, upd := range n.GetUpdate() {
		if upd.GetVal() == nil || upd.GetVal().GetValue() == nil {
			continue
		}
		scUpd := &schemapb.Update{
			Path:  FromGNMIPath(n.GetPrefix(), upd.GetPath()),
			Value: FromGNMITypedValue(upd.GetVal()),
		}
		sn.Update = append(sn.Update, scUpd)
	}
	return sn
}

func FromGNMIPath(pre, p *gnmi.Path) *schemapb.Path {
	if p == nil {
		return nil
	}
	r := &schemapb.Path{
		Origin: pre.GetOrigin(),
		Elem:   make([]*schemapb.PathElem, 0, len(pre.GetElem())+len(p.GetElem())),
		Target: pre.GetTarget(),
	}
	for _, pe := range pre.GetElem() {
		r.Elem = append(r.Elem, &schemapb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	for _, pe := range p.GetElem() {
		r.Elem = append(r.Elem, &schemapb.PathElem{
			Name: pe.GetName(),
			Key:  pe.GetKey(),
		})
	}
	return r
}

func FromGNMITypedValue(v *gnmi.TypedValue) *schemapb.TypedValue {
	log.Debugf("FromGNMITypedValue: %T : %v", v, v)
	if v == nil {
		return nil
	}
	log.Debugf("FromGNMITypedValue - Value: %T : %v", v.GetValue(), v.GetValue())
	switch v.GetValue().(type) {
	// case *gnmi.TypedValue:
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
			schemalf.Element = append(schemalf.Element, FromGNMITypedValue(e))
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
	default:
		log.Errorf("FromGNMITypedValue unhandled type: %T: %v", v, v)
		return nil
	}
	// return nil
}

func ToGNMIPath(p *schemapb.Path) *gnmi.Path {
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

// func toGNMITypedValue(v *schemapb.TypedValue) *gnmi.TypedValue {
// 	if v == nil {
// 		return nil
// 	}
// 	switch v.GetValue().(type) {
// 	case *schemapb.TypedValue_AnyVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AnyVal{AnyVal: v.GetAnyVal()},
// 		}
// 	case *schemapb.TypedValue_AsciiVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_AsciiVal{AsciiVal: v.GetAsciiVal()},
// 		}
// 	case *schemapb.TypedValue_BoolVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_BoolVal{BoolVal: v.GetBoolVal()},
// 		}
// 	case *schemapb.TypedValue_BytesVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_BytesVal{BytesVal: v.GetBytesVal()},
// 		}
// 	// case *schemapb.TypedValue_DecimalVal:
// 	// 	return &gnmi.TypedValue{
// 	// 		Value: &gnmi.TypedValue_DecimalVal{DecimalVal: v.GetDecimalVal()},
// 	// 	}
// 	// case *schemapb.TypedValue_FloatVal:
// 	// 	return &gnmi.TypedValue{
// 	// 		Value: &gnmi.TypedValue_FloatVal{FloatVal: v.GetFloatVal()},
// 	// 	}
// 	case *schemapb.TypedValue_IntVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_IntVal{IntVal: v.GetIntVal()},
// 		}
// 	case *schemapb.TypedValue_JsonIetfVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: v.GetJsonIetfVal()},
// 		}
// 	case *schemapb.TypedValue_JsonVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_JsonVal{JsonVal: v.GetJsonVal()},
// 		}
// 	case *schemapb.TypedValue_LeaflistVal:
// 		gnmilf := &gnmi.ScalarArray{
// 			Element: make([]*gnmi.TypedValue, 0, len(v.GetLeaflistVal().GetElement())),
// 		}
// 		for _, e := range v.GetLeaflistVal().GetElement() {
// 			gnmilf.Element = append(gnmilf.Element, toGNMITypedValue(e))
// 		}
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_LeaflistVal{LeaflistVal: gnmilf},
// 		}
// 	case *schemapb.TypedValue_ProtoBytes:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_ProtoBytes{ProtoBytes: v.GetProtoBytes()},
// 		}
// 	case *schemapb.TypedValue_StringVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_StringVal{StringVal: v.GetStringVal()},
// 		}
// 	case *schemapb.TypedValue_UintVal:
// 		return &gnmi.TypedValue{
// 			Value: &gnmi.TypedValue_UintVal{UintVal: v.GetUintVal()},
// 		}
// 	}
// 	return nil
// }
