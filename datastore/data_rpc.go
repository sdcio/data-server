package datastore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/schema-server/cache"
	"github.com/iptecharch/schema-server/datastore/ctree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/iptecharch/yang-parser/xpath"
	"github.com/iptecharch/yang-parser/xpath/grammars/expr"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func (d *Datastore) Get(ctx context.Context, req *schemapb.GetDataRequest, nCh chan *schemapb.GetDataResponse) error {
	defer close(nCh)

	var err error
	// validate that path(s) exist in the schema
	for _, p := range req.GetPath() {
		_, err = d.validatePath(ctx, p)
		if err != nil {
			return err
		}
	}

	// choose store(s)
	var stores []cachepb.Store
	switch req.GetDataType() {
	case schemapb.DataType_ALL:
		stores = []cachepb.Store{cachepb.Store_CONFIG, cachepb.Store_STATE}
	case schemapb.DataType_CONFIG:
		stores = []cachepb.Store{cachepb.Store_CONFIG}
	case schemapb.DataType_STATE:
		stores = []cachepb.Store{cachepb.Store_STATE}
	}

	// build target cache name
	name := req.GetName()
	if req.GetDatastore().GetName() != "" {
		name = fmt.Sprintf("%s/%s", req.GetName(), req.GetDatastore().GetName())
	}

	// convert schemapb paths to a string list
	paths := make([][]string, 0, len(req.GetPath()))
	for _, p := range req.GetPath() {
		paths = append(paths, utils.ToStrings(p, false, false))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, store := range stores {
		for upd := range d.cacheClient.ReadCh(ctx, name, store, paths) {
			scp, err := d.toPath(ctx, upd.GetPath())
			if err != nil {
				return err
			}
			tv, err := upd.Value()
			if err != nil {
				return err
			}
			notification := &schemapb.Notification{
				Timestamp: time.Now().UnixNano(),
				Update: []*schemapb.Update{{
					Path:  scp,
					Value: tv,
				}},
			}
			rsp := &schemapb.GetDataResponse{
				Notification: []*schemapb.Notification{notification},
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case nCh <- rsp:
			}
		}
	}
	return nil
}

func (d *Datastore) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {
	switch req.GetDatastore().GetType() {
	case schemapb.Type_MAIN:
		return nil, status.Error(codes.InvalidArgument, "cannot set fields in MAIN datastore")
	case schemapb.Type_CANDIDATE:
		//
		ok, err := d.cacheClient.HasCandidate(ctx, req.GetName(), req.GetDatastore().GetName())
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown candidate %s", req.GetDatastore().GetName())
		}

		replaces := make([]*schemapb.Update, 0, len(req.GetReplace()))
		updates := make([]*schemapb.Update, 0, len(req.GetUpdate()))
		// expand json/json_ietf values
		for _, upd := range req.GetReplace() {
			rs, err := d.expandUpdate(ctx, upd)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			replaces = append(replaces, rs...)
		}
		for _, upd := range req.GetUpdate() {
			rs, err := d.expandUpdate(ctx, upd)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			updates = append(updates, rs...)
		}
		// debugging
		if log.GetLevel() >= log.DebugLevel {
			for _, upd := range replaces {
				log.Debugf("expanded replace:\n%s", prototext.Format(upd))
			}
			for _, upd := range updates {
				log.Debugf("expanded update:\n%s", prototext.Format(upd))
			}
		}

		// validate individual updates
		for _, del := range req.GetDelete() {
			_, err = d.validatePath(ctx, del)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "delete path: %q validation failed: %v", del, err)
			}
		}

		for _, upd := range replaces {
			err = d.validateUpdate(ctx, upd)
			if err != nil {
				log.Debugf("replace %v validation failed: %v", upd, err)
				return nil, status.Errorf(codes.InvalidArgument, "replace: validation failed: %v", err)
			}
		}

		for _, upd := range updates {
			err = d.validateUpdate(ctx, upd)
			if err != nil {
				log.Debugf("update %v validation failed: %v", upd, err)
				return nil, status.Errorf(codes.InvalidArgument, "update: validation failed: %v", err)
			}
		}

		// insert/delete
		// the order of operations is delete, replace, update
		rsp := &schemapb.SetDataResponse{
			Response: make([]*schemapb.UpdateResult, 0,
				len(req.GetDelete())+len(req.GetReplace())+len(req.GetUpdate())),
		}

		name := fmt.Sprintf("%s/%s", req.GetName(), req.GetDatastore().GetName())
		dels := make([][]string, 0, len(req.GetDelete()))
		upds := make([]cache.Update, 0, len(req.GetUpdate())+len(req.GetReplace()))
		// deletes start
		for _, del := range req.GetDelete() {
			dels = append(dels, utils.ToStrings(del, false, false))
		}
		for _, changes := range [][]*schemapb.Update{replaces, updates} {
			for _, upd := range changes {
				cUpd, err := d.cacheClient.NewUpdate(upd)
				if err != nil {
					return nil, err
				}
				upds = append(upds, cUpd)
			}
		}
		err = d.cacheClient.Modify(ctx, name, cachepb.Store_CONFIG, dels, upds)
		if err != nil {
			return nil, err
		}

		// deletes start
		for _, del := range req.GetDelete() {
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: del,
				Op:   schemapb.UpdateResult_DELETE,
			})
		}
		// deletes end
		// replaces start
		for _, rep := range req.GetReplace() {
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: rep.GetPath(),
				Op:   schemapb.UpdateResult_REPLACE,
			})
		}
		// replaces end
		// updates start
		for _, upd := range req.GetUpdate() {
			rsp.Response = append(rsp.Response, &schemapb.UpdateResult{
				Path: upd.GetPath(),
				Op:   schemapb.UpdateResult_UPDATE,
			})
		}
		// updates end
		return rsp, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %v", req.GetDatastore().GetType())
	}
}

func (d *Datastore) Diff(ctx context.Context, req *schemapb.DiffRequest) (*schemapb.DiffResponse, error) {
	switch req.GetDatastore().GetType() {
	case schemapb.Type_MAIN:
		return nil, status.Errorf(codes.InvalidArgument, "must set a candidate datastore")
	case schemapb.Type_CANDIDATE:
		changes, err := d.cacheClient.GetChanges(ctx, req.GetName(), req.GetDatastore().GetName())
		if err != nil {
			return nil, err
		}
		diffRsp := &schemapb.DiffResponse{
			Name:      req.GetName(),
			Datastore: req.GetDatastore(),
			Diff:      make([]*schemapb.DiffUpdate, 0, len(changes)),
		}
		for _, change := range changes {
			switch {
			case change.Update != nil:
				candVal, err := change.Update.Value()
				if err != nil {
					return nil, err
				}

				// read value from main
				values := d.cacheClient.Read(ctx, req.GetName(), cachepb.Store_CONFIG, [][]string{change.Update.GetPath()})
				if len(values) == 0 {
					continue
				}
				mainVal, err := values[0].Value()
				if err != nil {
					return nil, err
				}

				// compare values
				if equalTypedValues(mainVal, candVal) {
					continue
				}
				// get path from schema server
				p, err := d.schemaClient.ToPath(ctx,
					&schemapb.ToPathRequest{
						PathElement: change.Update.GetPath(),
						Schema: &schemapb.Schema{
							Name:    d.config.Schema.Name,
							Vendor:  d.config.Schema.Vendor,
							Version: d.config.Schema.Version,
						},
					})
				if err != nil {
					return nil, err
				}
				diffup := &schemapb.DiffUpdate{
					Path:           p.GetPath(),
					MainValue:      mainVal,
					CandidateValue: candVal,
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
			case len(change.Delete) != 0:
				// read value from main
				values := d.cacheClient.Read(ctx, req.GetName(), cachepb.Store_CONFIG, [][]string{change.Delete})
				if len(values) == 0 {
					continue
				}
				val, err := values[0].Value()
				if err != nil {
					return nil, err
				}

				// get path from schema server
				p, err := d.schemaClient.ToPath(ctx,
					&schemapb.ToPathRequest{
						PathElement: change.Delete,
						Schema: &schemapb.Schema{
							Name:    d.config.Schema.Name,
							Vendor:  d.config.Schema.Vendor,
							Version: d.config.Schema.Version,
						},
					})
				if err != nil {
					return nil, err
				}

				diffup := &schemapb.DiffUpdate{
					Path:      p.GetPath(),
					MainValue: val,
				}
				diffRsp.Diff = append(diffRsp.Diff, diffup)
			}
		}
		return diffRsp, nil
	}
	return nil, status.Errorf(codes.InvalidArgument, "unknown datastore type %s", req.GetDatastore().GetType())
}

func (d *Datastore) Subscribe(req *schemapb.SubscribeRequest, stream schemapb.DataServer_SubscribeServer) error {
	return nil
}

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
		return fmt.Errorf("cannot set value on container %q object", obj.Container.Name)
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
			Path:   p,
			Schema: d.Schema().GetSchema(),
		})
}

func (d *Datastore) validateMustStatement(ctx context.Context, p *schemapb.Path, headTree *ctree.Tree, rsp *schemapb.GetSchemaResponse) (bool, error) {

	var mustStatements []*schemapb.MustStatement
	switch rsp.GetSchema().(type) {
	case *schemapb.GetSchemaResponse_Container:
		mustStatements = rsp.GetContainer().GetMustStatements()
	case *schemapb.GetSchemaResponse_Leaflist:
		mustStatements = rsp.GetLeaflist().GetMustStatements()
	case *schemapb.GetSchemaResponse_Field:
		mustStatements = rsp.GetField().GetMustStatements()
	}

	for _, must := range mustStatements {
		// extract actual must statement
		exprStr := must.Statement
		// init a ProgramBuilder
		prgbuilder := xpath.NewProgBuilder(exprStr)
		// init an ExpressionLexer
		lexer := expr.NewExprLex(exprStr, prgbuilder, nil)
		// parse the provided Must-Expression
		lexer.Parse()
		prog, err := lexer.CreateProgram(exprStr)
		if err != nil {
			return false, err
		}

		machine := xpath.NewMachine(exprStr, prog, exprStr)
		// create a context that takes the machine, but also also the references to the actual yang entrity.
		schema := &schemapb.Schema{Name: d.config.Schema.Name, Version: d.config.Schema.Version, Vendor: d.config.Schema.Vendor}
		// run the must statement evaluation virtual machine
		res1 := xpath.NewCtxFromCurrent(machine, p.Elem, headTree, schema, d.schemaClient, ctx).EnableValidation().Run()

		// retrieve the boolean result of the execution
		result, err := res1.GetBoolResult()
		if !result || err != nil {
			if err == nil {
				err = fmt.Errorf(must.Error)
			}
			return result, err
		}
	}
	return true, nil
}

func validateFieldValue(f *schemapb.LeafSchema, v any) error {
	// TODO: eval must statements
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
		case bool:
			return nil
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
	case "identityref":
		valid := false
		for _, vv := range lt.Values {
			if fmt.Sprintf("%s", v) == vv {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %q does not match identityRef type %q, must be one of [%s]", v, lt.TypeName, strings.Join(lt.Values, ", "))
		}
		return nil
	case "leafref":
		// TODO: does this need extra validation?
		return nil
	default:
		return fmt.Errorf("unhandled type %v", lt.GetType())
	}
}

func validateLeafListValue(ll *schemapb.LeafListSchema, v any) error {
	// TODO: validate Leaflist
	// TODO: eval must statements
	for _, must := range ll.MustStatements {
		_ = must
	}
	return validateLeafTypeValue(ll.GetType(), v)
}

func equalTypedValues(v1, v2 *schemapb.TypedValue) bool {
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
				if !equalTypedValues(v1.LeaflistVal.Element[i], v2.LeaflistVal.Element[i]) {
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

func (d *Datastore) expandUpdate(ctx context.Context, upd *schemapb.Update) ([]*schemapb.Update, error) {
	rsp, err := d.schemaClient.GetSchema(ctx,
		&schemapb.GetSchemaRequest{
			Path:   upd.GetPath(),
			Schema: d.Schema().GetSchema(),
		})
	if err != nil {
		return nil, err
	}
	switch rsp := rsp.GetSchema().(type) {
	case *schemapb.GetSchemaResponse_Container:
		log.Debugf("datastore %s: expanding update %v on container %q", d.config.Name, upd, rsp.Container.Name)
		var v interface{}
		err := json.Unmarshal(upd.GetValue().GetJsonVal(), &v)
		if err != nil {
			return nil, err
		}
		log.Debugf("datastore %s: update has jsonVal: %T, %v\n", d.config.Name, v, v)
		rs, err := d.expandContainerValue(ctx, upd.GetPath(), v, rsp)
		if err != nil {
			return nil, err
		}
		return rs, nil
	case *schemapb.GetSchemaResponse_Field:
		// TODO: Check if value is json and convert to String ?
		return []*schemapb.Update{upd}, nil
	case *schemapb.GetSchemaResponse_Leaflist:
		// TODO: Check if value is json and convert to String ?
		return []*schemapb.Update{upd}, nil
	}
	return nil, nil
}

func (d *Datastore) expandContainerValue(ctx context.Context, p *schemapb.Path, jv any, cs *schemapb.GetSchemaResponse_Container) ([]*schemapb.Update, error) {
	log.Debugf("expanding jsonVal %T | %v | %v", jv, jv, p)
	switch jv := jv.(type) {
	case string:
		v := strings.Trim(jv, "\"")
		return []*schemapb.Update{
			{
				Path: p,
				Value: &schemapb.TypedValue{
					Value: &schemapb.TypedValue_StringVal{StringVal: v},
				},
			},
		}, nil
	case map[string]any:
		upds := make([]*schemapb.Update, 0)
		// make sure all keys are present
		// and append them to path
		var keysInPath map[string]string
		if numElems := len(p.GetElem()); numElems > 0 {
			keysInPath = p.GetElem()[numElems-1].GetKey()
		}
		// handling keys in last element of the path or in the json value
		for _, k := range cs.Container.GetKeys() {
			if v, ok := jv[k.Name]; ok {
				log.Debugf("handling key %s", k.Name)
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and value", k.Name)
				}
				if p.GetElem()[len(p.GetElem())-1].Key == nil {
					p.GetElem()[len(p.GetElem())-1].Key = make(map[string]string)
				}
				p.GetElem()[len(p.GetElem())-1].Key[k.Name] = fmt.Sprintf("%v", v)
				continue
			}
			// if key is not in the value it must be set in the path
			if _, ok := keysInPath[k.Name]; !ok {
				return nil, fmt.Errorf("missing key %q from container %q", k.Name, cs.Container.Name)
			}
		}

		for k, v := range jv {
			if isKey(k, cs) {
				continue
			}
			item, ok := d.getItem(ctx, k, cs)
			if !ok {
				return nil, fmt.Errorf("unknown object %q under container %q", k, cs.Container.Name)
			}
			switch item := item.(type) {
			case *schemapb.LeafSchema: // field
				log.Debugf("handling field %s", item.Name)
				np := proto.Clone(p).(*schemapb.Path)
				np.Elem = append(np.Elem, &schemapb.PathElem{Name: item.Name})
				upd := &schemapb.Update{
					Path: np,
					Value: &schemapb.TypedValue{
						Value: &schemapb.TypedValue_StringVal{
							StringVal: fmt.Sprintf("%v", v),
						},
					},
				}
				upds = append(upds, upd)
			case *schemapb.LeafListSchema: // leaflist
				log.Debugf("TODO: handling leafList %s", item.Name)
			case string: // child container
				log.Debugf("handling child container %s", item)
				np := proto.Clone(p).(*schemapb.Path)
				np.Elem = append(np.Elem, &schemapb.PathElem{Name: item})
				rsp, err := d.schemaClient.GetSchema(ctx,
					&schemapb.GetSchemaRequest{
						Path:   np,
						Schema: d.Schema().GetSchema(),
					})
				if err != nil {
					return nil, err
				}
				switch rsp := rsp.GetSchema().(type) {
				case *schemapb.GetSchemaResponse_Container:
					rs, err := d.expandContainerValue(ctx, np, v, rsp)
					if err != nil {
						return nil, err
					}
					upds = append(upds, rs...)
				default:
					// should not happen
					return nil, fmt.Errorf("object %q is not a container", item)
				}
			default:
				return nil, fmt.Errorf("unknown object %q under container %q", k, cs.Container.Name)
			}
		}
		return upds, nil
	case []any:
		upds := make([]*schemapb.Update, 0)
		for _, v := range jv {
			np := proto.Clone(p).(*schemapb.Path)
			r, err := d.expandContainerValue(ctx, np, v, cs)
			if err != nil {
				return nil, err
			}
			upds = append(upds, r...)
		}
		return upds, nil
	default:
		log.Warnf("unexpected json type cast %T", jv)
		return nil, nil
	}
}

func (d *Datastore) getItem(ctx context.Context, s string, cs *schemapb.GetSchemaResponse_Container) (any, bool) {
	f, ok := getField(s, cs)
	if ok {
		return f, true
	}
	lfl, ok := getLeafList(s, cs)
	if ok {
		return lfl, true
	}
	c, ok := d.getChild(ctx, s, cs)
	if ok {
		return c, true
	}
	return nil, false
}

func isKey(s string, cs *schemapb.GetSchemaResponse_Container) bool {
	for _, k := range cs.Container.GetKeys() {
		if k.Name == s {
			return true
		}
	}
	return false
}

func getField(s string, cs *schemapb.GetSchemaResponse_Container) (*schemapb.LeafSchema, bool) {
	for _, f := range cs.Container.GetFields() {
		if f.Name == s {
			return f, true
		}
	}
	return nil, false
}

func getLeafList(s string, cs *schemapb.GetSchemaResponse_Container) (*schemapb.LeafListSchema, bool) {
	for _, lfl := range cs.Container.GetLeaflists() {
		if lfl.Name == s {
			return lfl, true
		}
	}
	return nil, false
}

func (d *Datastore) getChild(ctx context.Context, s string, cs *schemapb.GetSchemaResponse_Container) (string, bool) {
	for _, c := range cs.Container.GetChildren() {
		if c == s {
			return c, true
		}
	}
	if cs.Container.Name == "root" {
		for _, c := range cs.Container.GetChildren() {
			rsp, err := d.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
				Path:   &schemapb.Path{Elem: []*schemapb.PathElem{{Name: c}}},
				Schema: d.Schema().GetSchema(),
			})
			if err != nil {
				log.Errorf("Failed to get schema object %s: %v", c, err)
				return "", false
			}
			switch rsp := rsp.Schema.(type) {
			case *schemapb.GetSchemaResponse_Container:
				for _, child := range rsp.Container.GetChildren() {
					if child == s {
						return child, true
					}
				}
			default:
				continue
			}
		}
	}
	return "", false
}
