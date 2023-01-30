package datastore

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/iptecharch/schema-server/datastore/ctree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

func (d *Datastore) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	switch req.GetDataStore().GetType() {
	case schemapb.Type_CANDIDATE:
		d.m.RLock()
		defer d.m.RUnlock()
		cand, ok := d.candidates[req.GetDataStore().GetName()]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown candidate %s", req.GetDataStore().GetName())
		}
		rsp := &schemapb.GetDataResponse{
			Notification: make([]*schemapb.Notification, 0, len(req.GetPath())),
		}
		tempTree := &ctree.Tree{}
		log.Infof("reading from candidate")
		for _, p := range req.GetPath() {
			log.Infof("from head path %v", p)
			n, err := cand.head.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				log.Errorf("get from head err: %v", err)
				return nil, err
			}
			log.Infof("from head result %v", n)
			for _, nn := range n {
				err = tempTree.AddSchemaNotification(nn)
				if err != nil {
					return nil, err
				}
			}
			if len(n) > 0 {
				continue
			}
			log.Infof("from base path %v", p)
			bn, err := cand.base.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				return nil, err
			}

			log.Infof("from base result %v", bn)
			for _, nn := range bn {
				err = tempTree.AddSchemaNotification(nn)
				if err != nil {
					return nil, err
				}
			}
		}
		for _, p := range req.GetPath() {
			n, err := tempTree.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				return nil, err
			}
			log.Infof("from tempTree result %v", n)
			rsp.Notification = append(rsp.Notification, n...)
		}

		return rsp, nil
	case schemapb.Type_MAIN:
		rsp := &schemapb.GetDataResponse{
			Notification: make([]*schemapb.Notification, 0, len(req.GetPath())),
		}
		reqPaths := req.GetPath()
		if reqPaths == nil {
			reqPaths = []*schemapb.Path{{
				Elem: []*schemapb.PathElem{{Name: ""}},
			}}
		}
		for _, p := range reqPaths {
			nc, err := d.main.config.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				return nil, err
			}
			rsp.Notification = append(rsp.Notification, nc...)
			ns, err := d.main.state.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				return nil, err
			}
			rsp.Notification = append(rsp.Notification, ns...)
		}
		return rsp, nil
	}
	return nil, fmt.Errorf("unknown datastore type: %v", req.GetDataStore().GetType())
}

func (d *Datastore) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	switch req.GetDataStore().GetType() {
	case schemapb.Type_MAIN:
		return nil, status.Error(codes.InvalidArgument, "cannot set fields in MAIN datastore")
	case schemapb.Type_CANDIDATE:
		d.m.RLock()
		defer d.m.RUnlock()
		cand, ok := d.candidates[req.GetDataStore().GetName()]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown candidate %s", req.GetDataStore().GetName())
		}
		// validate individual updates
		var err error
		for _, del := range req.GetDelete() {
			_, err = d.validatePath(ctx, del)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%w", err)
			}
		}
		for _, upd := range req.GetReplace() {
			err = d.validateUpdate(ctx, upd)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%w", err)
			}
		}
		for _, upd := range req.GetUpdate() {
			err = d.validateUpdate(ctx, upd)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%w", err)
			}
		}

		// insert/delete
		// the order of operations is delete, replace, update
		rsp := &schemapb.SetDataResponse{
			Response: make([]*schemapb.UpdateResult, 0,
				len(req.GetDelete())+len(req.GetReplace())+len(req.GetUpdate())),
		}
		// deletes start
		for _, del := range req.GetDelete() {
			err = cand.head.DeletePath(del)
			if err != nil {
				return nil, err
			}
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: del,
				Op:   schemapb.UpdateResult_DELETE,
			})

		}
		cand.deletes = append(cand.deletes, req.GetDelete()...)
		// deletes end

		// replaces start
		for _, rep := range req.GetReplace() {
			err = cand.head.AddSchemaUpdate(rep)
			if err != nil {
				return nil, err
			}
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: rep.GetPath(),
				Op:   schemapb.UpdateResult_REPLACE,
			})
		}
		cand.replaces = append(cand.replaces, req.GetReplace()...)
		// replaces end
		// updates start
		for _, upd := range req.GetUpdate() {
			err = cand.head.AddSchemaUpdate(upd)
			if err != nil {
				return nil, err
			}
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: upd.GetPath(),
				Op:   schemapb.UpdateResult_UPDATE,
			})
		}
		cand.updates = append(cand.updates, req.GetUpdate()...)
		// updates end
		return rsp, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %v", req.GetDataStore().GetType())
	}
}

func (d *Datastore) Diff(ctx context.Context, req *schemapb.DiffRequest) (*schemapb.DiffResponse, error) {
	switch req.GetDataStore().GetType() {
	case schemapb.Type_MAIN:
		return nil, status.Errorf(codes.InvalidArgument, "must set a candidate datastore")
	case schemapb.Type_CANDIDATE:
		d.m.RLock()
		defer d.m.RUnlock()
		cand, ok := d.candidates[req.GetDataStore().GetName()]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown candidate %s", req.GetDataStore().GetName())
		}
		diffRsp := &schemapb.DiffResponse{
			Name:      req.GetName(),
			DataStore: req.GetDataStore(),
			Diff:      []*schemapb.DiffUpdate{},
		}
		for _, del := range cand.deletes {
			n, err := cand.base.GetPath(ctx, del, d.schemaClient, d.Schema())
			if err != nil {
				return nil, err
			}
			if len(n) == 0 {
				continue
			}
			if len(n[0].GetUpdate()) == 0 {
				continue
			}

			diffup := &schemapb.DiffUpdate{
				Path:      del,
				MainValue: n[0].GetUpdate()[0].GetValue(),
			}
			diffRsp.Diff = append(diffRsp.Diff, diffup)
		}
		for _, rep := range cand.replaces {
			n, err := cand.base.GetPath(ctx, rep.GetPath(), d.schemaClient, d.Schema())
			if err != nil {
				return nil, err
			}
			if len(n) == 0 || len(n[0].GetUpdate()) == 0 {
				diffup := &schemapb.DiffUpdate{
					Path:           rep.GetPath(),
					MainValue:      nil,
					CandidateValue: rep.GetValue(),
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
				continue
			}
			if len(n) != 0 && len(n[0].GetUpdate()) != 0 {
				if EqualTypedValues(n[0].GetUpdate()[0].GetValue(), rep.GetValue()) {
					continue
				}
				diffup := &schemapb.DiffUpdate{
					Path:           rep.GetPath(),
					MainValue:      n[0].GetUpdate()[0].GetValue(),
					CandidateValue: rep.GetValue(),
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
			}
		}
		for _, upd := range cand.updates {
			n, err := cand.base.GetPath(ctx, upd.GetPath(), d.schemaClient, d.Schema())
			if err != nil {
				return nil, err
			}
			if len(n) == 0 || len(n[0].GetUpdate()) == 0 {
				diffup := &schemapb.DiffUpdate{
					Path:           upd.GetPath(),
					MainValue:      nil,
					CandidateValue: upd.GetValue(),
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
				continue
			}
			if len(n) != 0 && len(n[0].GetUpdate()) != 0 {
				if EqualTypedValues(n[0].GetUpdate()[0].GetValue(), upd.GetValue()) {
					continue
				}
				diffup := &schemapb.DiffUpdate{
					Path:           upd.GetPath(),
					MainValue:      n[0].GetUpdate()[0].GetValue(),
					CandidateValue: upd.GetValue(),
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
			}
		}
		return diffRsp, nil
	}
	return nil, status.Errorf(codes.InvalidArgument, "unknown datastore type %s", req.GetDataStore().GetType())
}

func (d *Datastore) Subscribe() {}

func (d *Datastore) validateUpdate(ctx context.Context, upd *schemapb.Update) error {
	// 1.validate the path i.e check that the path exists
	// 2.validate that the value is compliant with the schema

	// 1. validate the path
	rsp, err := d.validatePath(ctx, upd.GetPath())
	if err != nil {
		return err
	}
	// 2. validate value
	val, err := utils.GetSchemaValue(upd.GetValue())
	if err != nil {
		return err
	}
	switch obj := rsp.GetSchema().(type) {
	case *schemapb.GetSchemaResponse_Container:
		return fmt.Errorf("cannot set value on container object")
	case *schemapb.GetSchemaResponse_Field:
		if obj.Field.IsState {
			return fmt.Errorf("cannot set state field: %v", obj.Field.Name)
		}
		err = validateFieldValue(obj.Field, val)
		if err != nil {
			return err
		}
	case *schemapb.GetSchemaResponse_Leaflist:
		err = validateLeafListValue(obj.Leaflist, val)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Datastore) validatePath(ctx context.Context, p *schemapb.Path) (*schemapb.GetSchemaResponse, error) {
	return d.schemaClient.GetSchema(ctx,
		&schemapb.GetSchemaRequest{
			Path: p,
			Schema: &schemapb.Schema{
				Name:    d.config.Schema.Name,
				Vendor:  d.config.Schema.Vendor,
				Version: d.config.Schema.Version,
			},
		})
}

func validateFieldValue(f *schemapb.LeafSchema, v any) error {
	// TODO: eval must statements
	for _, must := range f.MustStatements {
		_ = must
	}
	return validateLeafTypeValue(f.GetType(), v)
}

func validateLeafTypeValue(lt *schemapb.SchemaLeafType, v any) error {
	switch lt.GetType() {
	case "string":
		return nil
	case "int8":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseInt(v, 10, 8)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "int16":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseInt(v, 10, 16)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "int32":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "int64":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "uint8":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseUint(v, 10, 8)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "uint16":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "uint32":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "uint64":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "boolean":
		switch v := v.(type) {
		case string:
			switch {
			case v == "true":
			case v == "false":
			default:
				return fmt.Errorf("value %v must be a boolean", v)
			}
		default:
			return fmt.Errorf("unexpected casted type %T in %v", v, lt.GetType())
		}
		return nil
	case "enumeration":
		valid := false
		for _, vv := range lt.Values {
			if fmt.Sprintf("%s", v) == vv {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %q does not match enum type %q, must be one of [%s]", v, lt.TypeName, strings.Join(lt.Values, ", "))
		}
		return nil
	case "union":
		valid := false
		for _, ut := range lt.GetUnionTypes() {
			err := validateLeafTypeValue(ut, v)
			if err == nil {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %v does not match union type %v", v, lt.TypeName)
		}
		return nil
	default:
		return fmt.Errorf("unhandled type %v", lt.GetType())
	}
}

func validateLeafListValue(ll *schemapb.LeafListSchema, v any) error {
	// TODO: validate Leaflist
	return validateLeafTypeValue(ll.GetType(), v)
}

func EqualTypedValues(v1, v2 *schemapb.TypedValue) bool {
	if v1 == nil {
		return v2 == nil
	}
	if v2 == nil {
		return v1 == nil
	}

	switch v1 := v1.GetValue().(type) {
	case *schemapb.TypedValue_AnyVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_AnyVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if v1.AnyVal == nil && v2.AnyVal == nil {
				return true
			}
			if v1.AnyVal == nil || v2.AnyVal == nil {
				return false
			}
			if v1.AnyVal.GetTypeUrl() != v2.AnyVal.GetTypeUrl() {
				return false
			}
			return bytes.Equal(v1.AnyVal.GetValue(), v2.AnyVal.GetValue())
		default:
			return false
		}
	case *schemapb.TypedValue_AsciiVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_AsciiVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.AsciiVal == v2.AsciiVal
		default:
			return false
		}
	case *schemapb.TypedValue_BoolVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_BoolVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.BoolVal == v2.BoolVal
		default:
			return false
		}
	case *schemapb.TypedValue_BytesVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_BytesVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.BytesVal, v2.BytesVal)
		default:
			return false
		}
	case *schemapb.TypedValue_DecimalVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_DecimalVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if v1.DecimalVal.GetDigits() != v2.DecimalVal.GetDigits() {
				return false
			}
			return v1.DecimalVal.GetPrecision() == v2.DecimalVal.GetPrecision()
		default:
			return false
		}
	case *schemapb.TypedValue_FloatVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_FloatVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.FloatVal == v2.FloatVal
		default:
			return false
		}
	case *schemapb.TypedValue_IntVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_IntVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.IntVal == v2.IntVal
		default:
			return false
		}
	case *schemapb.TypedValue_JsonIetfVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_JsonIetfVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.JsonIetfVal, v2.JsonIetfVal)
		default:
			return false
		}
	case *schemapb.TypedValue_JsonVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_JsonVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.JsonVal, v2.JsonVal)
		default:
			return false
		}
	case *schemapb.TypedValue_LeaflistVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_LeaflistVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			if len(v1.LeaflistVal.GetElement()) != len(v2.LeaflistVal.GetElement()) {
				return false
			}
			for i := range v1.LeaflistVal.GetElement() {
				if !EqualTypedValues(v1.LeaflistVal.Element[i], v2.LeaflistVal.Element[i]) {
					return false
				}
			}
		default:
			return false
		}
	case *schemapb.TypedValue_ProtoBytes:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_ProtoBytes:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return bytes.Equal(v1.ProtoBytes, v2.ProtoBytes)
		default:
			return false
		}
	case *schemapb.TypedValue_StringVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_StringVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.StringVal == v2.StringVal
		default:
			return false
		}
	case *schemapb.TypedValue_UintVal:
		switch v2 := v2.GetValue().(type) {
		case *schemapb.TypedValue_UintVal:
			if v1 == nil && v2 == nil {
				return true
			}
			if v1 == nil || v2 == nil {
				return false
			}
			return v1.UintVal == v2.UintVal
		default:
			return false
		}
	}
	return true
}
