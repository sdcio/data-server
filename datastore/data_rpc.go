package datastore

import (
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
				err = tempTree.AddSchemaUpdate(nn)
				if err != nil {
					return nil, err
				}
			}
			// bbbb, err := tempTree.PrettyJSON()
			// if err != nil {
			// 	panic(err)
			// }
			// fmt.Println("head\n", string(bbbb))
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
				err = tempTree.AddSchemaUpdate(nn)
				if err != nil {
					return nil, err
				}
			}
			// bbbb, err = tempTree.PrettyJSON()
			// if err != nil {
			// 	panic(err)
			// }
			// fmt.Println("after head\n", string(bbbb))
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
			n, err := d.main.GetPath(ctx, p, d.schemaClient, d.config.Schema)
			if err != nil {
				return nil, err
			}
			rsp.Notification = append(rsp.Notification, n...)
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

		for _, rep := range req.GetReplace() {
			err = cand.head.Insert(rep)
			if err != nil {
				return nil, err
			}
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: rep.GetPath(),
				Op:   schemapb.UpdateResult_REPLACE,
			})
		}
		cand.replaces = append(cand.replaces, req.GetReplace()...)

		for _, upd := range req.GetUpdate() {
			err = cand.head.Insert(upd)
			if err != nil {
				return nil, err
			}
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: upd.GetPath(),
				Op:   schemapb.UpdateResult_UPDATE,
			})
		}
		cand.updates = append(cand.updates, req.GetUpdate()...)
		return rsp, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %v", req.GetDataStore().GetType())
	}
}

func (d *Datastore) Diff(ctx context.Context, req *schemapb.DiffRequest, dsType *schemapb.DataStore) (*schemapb.DiffResponse, error) {
	return nil, nil
}

func (d *Datastore) Subscribe() {}

func (d *Datastore) validateUpdate(ctx context.Context, upd *schemapb.Update) error {
	// 1.validate the path i.e check that the path exists
	// 2.validate that the value is compliant with the schema

	// 1. validate the path
	rsp, err := d.validatePath(ctx, upd.GetPath())
	fmt.Println(err)
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
		_ = obj
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
