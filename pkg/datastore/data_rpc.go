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

package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sdcio/cache/proto/cachepb"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/sdcio/yang-parser/xpath"
	"github.com/sdcio/yang-parser/xpath/grammars/expr"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/datastore/jbuilderv2"
	"github.com/sdcio/data-server/pkg/utils"
)

const (
	// to be used for candidates created without an owner
	DefaultOwner = "__sdcio"
)

func (d *Datastore) Get(ctx context.Context, req *sdcpb.GetDataRequest, nCh chan *sdcpb.GetDataResponse) error {
	defer close(nCh)
	switch req.GetDatastore().GetType() {
	case sdcpb.Type_MAIN:
	case sdcpb.Type_CANDIDATE:
	case sdcpb.Type_INTENDED:
		if req.GetDataType() == sdcpb.DataType_STATE {
			return status.Error(codes.InvalidArgument, "cannot query STATE data from INTENDED store")
		}
	}

	switch req.GetEncoding() {
	case sdcpb.Encoding_STRING:
	case sdcpb.Encoding_JSON:
	case sdcpb.Encoding_JSON_IETF:
		return fmt.Errorf("not implemented")
	case sdcpb.Encoding_PROTO:
	default:
		return fmt.Errorf("unknown encoding: %v", req.GetEncoding())
	}

	var err error
	// validate that path(s) exist in the schema
	for _, p := range req.GetPath() {
		err = d.validatePath(ctx, p)
		if err != nil {
			return err
		}
	}

	// build target cache name
	name := req.GetName()
	if req.GetDatastore().GetName() != "" {
		name = fmt.Sprintf("%s/%s", req.GetName(), req.GetDatastore().GetName())
	}

	// convert sdcpb paths to a string list
	paths := make([][]string, 0, len(req.GetPath()))
	for _, p := range req.GetPath() {
		paths = append(paths, utils.ToStrings(p, false, false))
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	switch req.GetEncoding() {
	case sdcpb.Encoding_STRING:
		err = d.handleGetDataUpdatesSTRING(ctx, name, req, paths, nCh)
	case sdcpb.Encoding_JSON:
		err = d.handleGetDataUpdatesJSON(ctx, name, req, paths, nCh)
	case sdcpb.Encoding_JSON_IETF:
	case sdcpb.Encoding_PROTO:
		err = d.handleGetDataUpdatesPROTO(ctx, name, req, paths, nCh)
	}
	if err != nil {
		return err
	}
	return nil
}

func (d *Datastore) handleGetDataUpdatesSTRING(ctx context.Context, name string, req *sdcpb.GetDataRequest, paths [][]string, out chan *sdcpb.GetDataResponse) error {
NEXT_STORE:
	for _, store := range getStores(req) {
		in := d.cacheClient.ReadCh(ctx, name, &cache.Opts{
			Store:    store,
			Owner:    req.GetDatastore().GetOwner(),
			Priority: req.GetDatastore().GetPriority(),
		}, paths, 0)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case upd, ok := <-in:
				//log.Debugf("ds=%s read path=%v from store=%v: %v", name, paths, store, upd)
				if !ok {
					continue NEXT_STORE
				}
				if len(upd.GetPath()) == 0 {
					continue
				}
				scp, err := d.toPath(ctx, upd.GetPath())
				if err != nil {
					return err
				}
				switch len(scp.GetElem()) {
				case 0:
					continue
				case 1:
					if scp.GetElem()[0].GetName() == "" {
						continue
					}
				}
				tv, err := upd.Value()
				if err != nil {
					return err
				}
				notification := &sdcpb.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*sdcpb.Update{{
						Path:  scp,
						Value: tv,
					}},
				}
				rsp := &sdcpb.GetDataResponse{
					Notification: []*sdcpb.Notification{notification},
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- rsp:
				}
			}
		}
	}
	return nil
}

func (d *Datastore) handleGetDataUpdatesJSON(ctx context.Context, name string, req *sdcpb.GetDataRequest, paths [][]string, out chan *sdcpb.GetDataResponse) error {
	jbuilder := jbuilderv2.New(d.getValidationClient())
	rs := make(map[string]any)
	now := time.Now().UnixNano()

	for _, store := range getStores(req) {
		in := d.cacheClient.ReadCh(ctx, name, &cache.Opts{
			Store:    store,
			Owner:    req.GetDatastore().GetOwner(),
			Priority: req.GetDatastore().GetPriority(),
		}, paths, 0)
	OUTER:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case upd, ok := <-in:
				if !ok {
					break OUTER
				}

				if len(upd.GetPath()) == 0 {
					continue
				}

				scp, err := d.toPath(ctx, upd.GetPath())
				if err != nil {
					return err
				}
				switch len(scp.GetElem()) {
				case 0:
					continue
				case 1:
					if scp.GetElem()[0].GetName() == "" {
						continue
					}
				}
				tv, err := upd.Value()
				if err != nil {
					return err
				}
				err = jbuilder.AddUpdate(ctx, rs, scp, tv)
				if err != nil {
					err = fmt.Errorf("failed json builder:path=%s, v=%v, err=%v", scp, tv, err)
					return err
				}
			}
		}
	}
	// marshal map into JSON bytes
	b, err := json.Marshal(rs)
	if err != nil {
		err = fmt.Errorf("failed json builder indent : %v", err)
		log.Error(err)
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- &sdcpb.GetDataResponse{
		Notification: []*sdcpb.Notification{
			{
				Timestamp: now,
				Update: []*sdcpb.Update{{
					Value: &sdcpb.TypedValue{Value: &sdcpb.TypedValue_JsonVal{JsonVal: b}},
				}},
			},
		},
	}:
	}
	return nil
}

func (d *Datastore) handleGetDataUpdatesPROTO(ctx context.Context, name string, req *sdcpb.GetDataRequest, paths [][]string, out chan *sdcpb.GetDataResponse) error {
NEXT_STORE:
	for _, store := range getStores(req) {
		in := d.cacheClient.ReadCh(ctx, name, &cache.Opts{
			Store:    store,
			Owner:    req.GetDatastore().GetOwner(),
			Priority: req.GetDatastore().GetPriority(),
		}, paths, 0)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case upd, ok := <-in:
				//log.Debugf("ds=%s read path=%v from store=%v: %v", name, paths, store, upd)
				if !ok {
					continue NEXT_STORE
				}

				if len(upd.GetPath()) == 0 {
					continue
				}
				scp, err := d.toPath(ctx, upd.GetPath())
				if err != nil {
					return err
				}
				switch len(scp.GetElem()) {
				case 0:
					continue
				case 1:
					if scp.GetElem()[0].GetName() == "" {
						continue
					}
				}
				tv, err := upd.Value()
				if err != nil {
					return err
				}
				ctv, err := d.convertTypedValueToProto(ctx, scp, tv)
				if err != nil {
					return err
				}
				notification := &sdcpb.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*sdcpb.Update{{
						Path:  scp,
						Value: ctv,
					}},
				}
				rsp := &sdcpb.GetDataResponse{
					Notification: []*sdcpb.Notification{notification},
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- rsp:
				}
			}
		}
	}
	return nil
}

func (d *Datastore) Set(ctx context.Context, req *sdcpb.SetDataRequest) (*sdcpb.SetDataResponse, error) {
	switch req.GetDatastore().GetType() {
	case sdcpb.Type_MAIN:
		return nil, status.Error(codes.InvalidArgument, "cannot set fields in MAIN datastore")
	case sdcpb.Type_INTENDED:
		return nil, status.Error(codes.InvalidArgument, "cannot set fields in INTENDED datastore")
	case sdcpb.Type_CANDIDATE:
		return d.setCandidate(ctx, req, true)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown datastore %v", req.GetDatastore().GetType())
	}
}

func (d *Datastore) Diff(ctx context.Context, req *sdcpb.DiffRequest) (*sdcpb.DiffResponse, error) {
	switch req.GetDatastore().GetType() {
	case sdcpb.Type_MAIN:
		return nil, status.Errorf(codes.InvalidArgument, "must set a candidate datastore")
	case sdcpb.Type_CANDIDATE:
		changes, err := d.cacheClient.GetChanges(ctx, req.GetName(), req.GetDatastore().GetName())
		if err != nil {
			return nil, err
		}
		// TODO: replace with a get candidate method
		cands, err := d.Candidates(ctx)
		if err != nil {
			return nil, err
		}
		diffRsp := &sdcpb.DiffResponse{
			Name:      req.GetName(),
			Datastore: req.GetDatastore(),
			Diff:      make([]*sdcpb.DiffUpdate, 0, len(changes)),
		}
		// TODO: replace with a get candidate method
		for _, cand := range cands {
			if cand.GetName() == req.GetDatastore().GetName() {
				diffRsp.Datastore.Owner = cand.GetOwner()
				diffRsp.Datastore.Priority = cand.GetPriority()
				break
			}
		}

		for _, change := range changes {
			switch {
			case change.Update != nil:
				candVal, err := change.Update.Value()
				if err != nil {
					return nil, err
				}

				// read value from main
				values := d.cacheClient.Read(ctx, req.GetName(), &cache.Opts{
					Store: cachepb.Store_CONFIG,
				}, [][]string{change.Update.GetPath()}, 0)
				log.Debugf("read values %v: %v", change.Update.GetPath(), values)
				switch len(values) {
				case 0: // value does not exist in main
					p, err := d.schemaClient.ToPath(ctx,
						&sdcpb.ToPathRequest{
							PathElement: change.Update.GetPath(),
							Schema: &sdcpb.Schema{
								Name:    d.config.Schema.Name,
								Vendor:  d.config.Schema.Vendor,
								Version: d.config.Schema.Version,
							},
						})
					if err != nil {
						return nil, err
					}
					diffup := &sdcpb.DiffUpdate{
						Path:           p.GetPath(),
						CandidateValue: candVal,
					}
					diffRsp.Diff = append(diffRsp.Diff, diffup)
				default:
					mainVal, err := values[0].Value()
					if err != nil {
						return nil, err
					}
					log.Debugf("read value %v: %v", change.Update.GetPath(), mainVal)
					log.Debugf("path=%v: main=%v, cand=%v", change.Update.GetPath(), mainVal, candVal)
					// compare values
					if utils.EqualTypedValues(mainVal, candVal) {
						continue
					}
					// get path from schema server
					p, err := d.schemaClient.ToPath(ctx,
						&sdcpb.ToPathRequest{
							PathElement: change.Update.GetPath(),
							Schema: &sdcpb.Schema{
								Name:    d.config.Schema.Name,
								Vendor:  d.config.Schema.Vendor,
								Version: d.config.Schema.Version,
							},
						})
					if err != nil {
						return nil, err
					}
					diffup := &sdcpb.DiffUpdate{
						Path:           p.GetPath(),
						MainValue:      mainVal,
						CandidateValue: candVal,
					}
					diffRsp.Diff = append(diffRsp.Diff, diffup)
				}
				if len(values) == 0 {
					continue // TODO: set empty value as main
				}

			case len(change.Delete) != 0:
				// read value from main
				values := d.cacheClient.Read(ctx, req.GetName(), &cache.Opts{
					Store: cachepb.Store_CONFIG,
				}, [][]string{change.Delete}, 0)
				if len(values) == 0 {
					continue
				}
				val, err := values[0].Value()
				if err != nil {
					return nil, err
				}

				// get path from schema server
				p, err := d.schemaClient.ToPath(ctx,
					&sdcpb.ToPathRequest{
						PathElement: change.Delete,
						Schema: &sdcpb.Schema{
							Name:    d.config.Schema.Name,
							Vendor:  d.config.Schema.Vendor,
							Version: d.config.Schema.Version,
						},
					})
				if err != nil {
					return nil, err
				}

				diffup := &sdcpb.DiffUpdate{
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

func (d *Datastore) Subscribe(req *sdcpb.SubscribeRequest, stream sdcpb.DataServer_SubscribeServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	var err error
	for _, subsc := range req.GetSubscription() {
		err := d.doSubscribeOnce(ctx, subsc, stream)
		if err != nil {
			return err
		}
	}
	err = stream.Send(&sdcpb.SubscribeResponse{
		Response: &sdcpb.SubscribeResponse_SyncResponse{
			SyncResponse: true,
		},
	})
	if err != nil {
		return err
	}
	// start periodic gets, TODO: optimize using cache RPC
	wg := new(sync.WaitGroup)
	wg.Add(len(req.GetSubscription()))
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	for _, subsc := range req.GetSubscription() {
		go func(subsc *sdcpb.Subscription) {
			ticker := time.NewTicker(time.Duration(subsc.GetSampleInterval()))
			defer ticker.Stop()
			defer wg.Done()
			for {
				select {
				case <-doneCh:
					return
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case <-ticker.C:
					err := d.doSubscribeOnce(ctx, subsc, stream)
					if err != nil {
						errCh <- err
						close(doneCh)
						return
					}
				}
			}
		}(subsc)
	}
	wg.Wait()
	return nil
}

func (d *Datastore) validateUpdate(ctx context.Context, upd *sdcpb.Update) error {
	// 1.validate the path i.e check that the path exists
	// 2.validate that the value is compliant with the schema

	// 1. validate the path
	rsp, err := d.getSchema(ctx, upd.GetPath())
	if err != nil {
		return err
	}
	// 2. convert value to its YANG type
	upd.Value, err = convertTypedValueToYANGType(rsp.GetSchema(), upd.GetValue())
	if err != nil {
		return err
	}
	// 2. validate value
	val, err := utils.GetSchemaValue(upd.GetValue())
	if err != nil {
		return err
	}
	switch obj := rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		if !pathIsKeyAsLeaf(upd.GetPath()) {
			return fmt.Errorf("cannot set value on container %q object", obj.Container.Name)
		}
		// TODO: validate key as leaf
	case *sdcpb.SchemaElem_Field:
		if obj.Field.IsState {
			return fmt.Errorf("cannot set state field: %v", obj.Field.Name)
		}
		err = validateFieldValue(obj.Field, val)
		if err != nil {
			return err
		}
	case *sdcpb.SchemaElem_Leaflist:
		err = validateLeafListValue(obj.Leaflist, val)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Datastore) validateMustStatement(ctx context.Context, candidateName string, p *sdcpb.Path, normalizePaths bool) (bool, error) {
	// normalizedPaths will contain the provided path. If the last path.elems contains one or more keys, these will be
	// taken and appended to the path. The must statements have to be checked for all of the key elements.
	var normalizedPaths = make([]*sdcpb.Path, 0, 1)

	// this is to massage for instance
	// /bfd/subinterface[id=ethernet-1.25] -> /bfd/subinterface[id=ethernet-1.25]/id
	// because we need to resolve down to id, to retrieve the relevant must statements
	// further there can be more then just a single key.
	if len(p.Elem[len(p.Elem)-1].Key) > 0 && normalizePaths {
		for k := range p.GetElem()[len(p.Elem)-1].GetKey() {
			// clone p as new path
			newPath := proto.Clone(p).(*sdcpb.Path)
			// take the key attribute name and add it as the new path.elem
			newPath.Elem = append(newPath.Elem, &sdcpb.PathElem{Name: k})
			// add the result to the normalized Paths
			normalizedPaths = append(normalizedPaths, newPath)
		}
	} else {
		// no keys attached to last path.elem, simply add the path
		normalizedPaths = append(normalizedPaths, p)
	}

	for _, checkPath := range normalizedPaths {
		rsp, err := d.getSchema(ctx, checkPath)
		if err != nil {
			return false, err
		}

		var mustStatements []*sdcpb.MustStatement
		switch rsp.GetSchema().Schema.(type) {
		case *sdcpb.SchemaElem_Container:
			mustStatements = rsp.Schema.GetContainer().GetMustStatements()
		case *sdcpb.SchemaElem_Leaflist:
			mustStatements = rsp.Schema.GetLeaflist().GetMustStatements()
		case *sdcpb.SchemaElem_Field:
			mustStatements = rsp.Schema.GetField().GetMustStatements()
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

			// run the must statement evaluation virtual machine
			res1 := xpath.NewCtxFromCurrent(ctx, machine, p.Elem, d.getValidationClient(), candidateName).Run()
			// retrieve the boolean result of the execution
			result, err := res1.GetBoolResult()
			if !result || err != nil {
				if err == nil {
					err = fmt.Errorf(must.Error)
				}
				if strings.Contains(err.Error(), "Stack underflow") {
					log.Warnf("stack underflow error: path=%v, mustExpr=%s", checkPath, exprStr)
					continue
				}
				return result, err
			}
		}
	}
	return true, nil
}

func validateFieldValue(f *sdcpb.LeafSchema, v any) error {
	return validateLeafTypeValue(f.GetType(), v)
}

func validateLeafTypeValue(lt *sdcpb.SchemaLeafType, v any) error {
	switch lt.GetType() {
	case "string":
		// TODO: validate length and range
		return nil
	case "int8":
		switch v := v.(type) {
		case string:
			_, err := strconv.ParseInt(v, 10, 8)
			if err != nil {
				return err
			}
		case int64:
			if v > math.MaxInt8 || v < math.MinInt8 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
		case int64:
			if v > math.MaxInt16 || v < math.MinInt16 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
		case int64:
			if v > math.MaxInt32 || v < math.MinInt32 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
		case uint64:
			if v > math.MaxUint8 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
		case uint64:
			if v > math.MaxUint16 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
		case uint64:
			if v > math.MaxUint32 {
				return fmt.Errorf("value %v out of bound for type %s", v, lt.GetType())
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
			_, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("value %v must be a boolean: %v", v, err)
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
	case "decimal64":
		switch v := v.(type) {
		case float64: // if it's a float64 then it's a valid decimal64
		case string:
			if c := strings.Count(v, "."); c == 0 || c > 1 {
				return fmt.Errorf("value %q is not a valid Decimal64", v)
			}
		default:
			return fmt.Errorf("unexpected type for a Decimal64 value %q: %T", v, v)
		}
		return nil
	case "leafref":
		// TODO: does this need extra validation?
		return nil
	default:
		return fmt.Errorf("unhandled type %v for value %q", lt.GetType(), v)
	}
}

func validateLeafListValue(ll *sdcpb.LeafListSchema, v any) error {
	// TODO: validate Leaflist
	// TODO: eval must statements
	for _, must := range ll.MustStatements {
		_ = must
	}
	return validateLeafTypeValue(ll.GetType(), v)
}

func (d *Datastore) expandUpdate(ctx context.Context, upd *sdcpb.Update, includeKeysAsLeaf bool) ([]*sdcpb.Update, error) {
	upds := make([]*sdcpb.Update, 0)
	if includeKeysAsLeaf {
		// expand update path if it contains keys
		intUpd, err := d.expandUpdateKeysAsLeaf(ctx, upd)
		if err != nil {
			return nil, err
		}
		upds = append(upds, intUpd...)
	}
	rsp, err := d.schemaClient.GetSchema(ctx,
		&sdcpb.GetSchemaRequest{
			Path:   upd.GetPath(),
			Schema: d.Schema().GetSchema(),
		})
	if err != nil {
		return nil, err
	}

	switch rsp := rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		log.Debugf("datastore %s: expanding update %v on container %q", d.config.Name, upd, rsp.Container.Name)
		var v interface{}
		var err error
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_JsonIetfVal:
			err = json.Unmarshal(upd.GetValue().GetJsonIetfVal(), &v)
		case *sdcpb.TypedValue_JsonVal:
			err = json.Unmarshal(upd.GetValue().GetJsonVal(), &v)
		default:
			return []*sdcpb.Update{upd}, nil
		}
		if err != nil {
			return nil, err
		}
		log.Debugf("datastore %s: update has jsonVal: %T, %v\n", d.config.Name, v, v)
		rs, err := d.expandContainerValue(ctx, upd.GetPath(), v, rsp, includeKeysAsLeaf)
		if err != nil {
			return nil, err
		}
		upds := append(upds, rs...)
		return upds, nil
	case *sdcpb.SchemaElem_Field:
		var v interface{}
		var err error

		var jsonValue []byte
		switch upd.GetValue().Value.(type) {
		case *sdcpb.TypedValue_JsonVal:
			jsonValue = upd.GetValue().GetJsonVal()
		case *sdcpb.TypedValue_JsonIetfVal:
			jsonValue = upd.GetValue().GetJsonIetfVal()
		}

		// process value
		if jsonValue != nil {
			err = json.Unmarshal(jsonValue, &v)
			if err != nil {
				return nil, err
			}
			switch v := v.(type) {
			case string:
				upd.Value = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
			}
		}

		// TODO: Check if value is json and convert to String ?
		upds = append(upds, upd)
		return upds, nil
	case *sdcpb.SchemaElem_Leaflist:
		// TODO: Check if value is json and convert to String ?
		upds = append(upds, upd)
		return upds, nil
	}
	return nil, nil
}

func (d *Datastore) expandUpdates(ctx context.Context, updates []*sdcpb.Update, includeKeysAsLeaf bool) ([]*sdcpb.Update, error) {
	outUpdates := make([]*sdcpb.Update, 0, len(updates))
	for _, upd := range updates {
		expUpds, err := d.expandUpdate(ctx, upd, includeKeysAsLeaf)
		if err != nil {
			return nil, err
		}
		outUpdates = append(outUpdates, expUpds...)
	}
	return outUpdates, nil
}

func (d *Datastore) expandUpdateKeysAsLeaf(ctx context.Context, upd *sdcpb.Update) ([]*sdcpb.Update, error) {
	upds := make([]*sdcpb.Update, 0)
	// expand update path if it contains keys
	for i, pe := range upd.GetPath().GetElem() {
		if len(pe.GetKey()) == 0 {
			continue
		}
		//
		for k, v := range pe.GetKey() {
			intUpd := &sdcpb.Update{
				Path: &sdcpb.Path{
					Elem: make([]*sdcpb.PathElem, 0, i+1+1),
				},
			}
			for j := 0; j <= i; j++ {
				intUpd.Path.Elem = append(intUpd.Path.Elem,
					&sdcpb.PathElem{
						Name: upd.GetPath().GetElem()[j].GetName(),
						Key:  upd.GetPath().GetElem()[j].GetKey(),
					},
				)
			}
			intUpd.Path.Elem = append(intUpd.Path.Elem, &sdcpb.PathElem{Name: k})
			intUpd.Value = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: v}}
			upds = append(upds, intUpd)
		}
	}
	return upds, nil
}

func (d *Datastore) expandContainerValue(ctx context.Context, p *sdcpb.Path, jv any, cs *sdcpb.SchemaElem_Container, includeKeysAsLeaf bool) ([]*sdcpb.Update, error) {
	log.Debugf("expanding jsonVal %T | %v | %v", jv, jv, p)
	switch jv := jv.(type) {
	case string:
		v := strings.Trim(jv, "\"")
		return []*sdcpb.Update{
			{
				Path: p,
				Value: &sdcpb.TypedValue{
					Value: &sdcpb.TypedValue_StringVal{StringVal: v},
				},
			},
		}, nil
	case map[string]any:
		upds := make([]*sdcpb.Update, 0)
		// make sure all keys are present
		// and append them to path
		var keysInPath map[string]string
		if numElems := len(p.GetElem()); numElems > 0 {
			keysInPath = p.GetElem()[numElems-1].GetKey()
		}
		// make sure all keys exist either in the JSON value or
		// in the path but NOT in both and build keySet
		keySet := map[string]string{}
		for _, k := range cs.Container.GetKeys() {
			if v, ok := jv[k.Name]; ok {
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and JSON value", k.Name)
				}
				keySet[k.Name] = fmt.Sprintf("%v", v)
				continue
			}
			if v, ok := keysInPath[k.Name]; ok {
				keySet[k.Name] = v
				continue
			}
			return nil, fmt.Errorf("missing key %s in element %s", k.Name, cs.Container.GetName())
		}
		// handling keys in last element of the path or in the json value
		for _, k := range cs.Container.GetKeys() {
			if v, ok := jv[k.Name]; ok {
				log.Debugf("handling key %s", k.Name)
				if _, ok := keysInPath[k.Name]; ok {
					return nil, fmt.Errorf("key %q is present in both the path and JSON value", k.Name)
				}
				if p.GetElem()[len(p.GetElem())-1].Key == nil {
					p.GetElem()[len(p.GetElem())-1].Key = make(map[string]string)
				}
				p.GetElem()[len(p.GetElem())-1].Key = keySet
				if includeKeysAsLeaf {
					np := proto.Clone(p).(*sdcpb.Path)
					np.Elem = append(np.Elem, &sdcpb.PathElem{Name: k.Name})
					upd := &sdcpb.Update{
						Path: np,
						Value: &sdcpb.TypedValue{
							Value: &sdcpb.TypedValue_StringVal{
								StringVal: fmt.Sprintf("%v", v),
							},
						},
					}
					upds = append(upds, upd)
				}
				continue
			}
			// if key is not in the value it must be set in the path
			if _, ok := keysInPath[k.Name]; !ok {
				return nil, fmt.Errorf("missing key %q from list %q", k.Name, cs.Container.Name)
			}
		}
		//
		for k, v := range jv {
			if isKey(k, cs) {
				continue
			}
			item, ok := d.getItem(ctx, k, cs)
			if !ok {
				return nil, fmt.Errorf("unknown object %q under container %q", k, cs.Container.Name)
			}
			switch item := item.(type) {
			case *sdcpb.LeafSchema: // field
				log.Debugf("handling field %s", item.Name)
				np := proto.Clone(p).(*sdcpb.Path)
				np.Elem = append(np.Elem, &sdcpb.PathElem{Name: item.Name})
				upd := &sdcpb.Update{
					Path: np,
					Value: &sdcpb.TypedValue{
						Value: &sdcpb.TypedValue_StringVal{
							StringVal: fmt.Sprintf("%v", v),
						},
					},
				}
				upds = append(upds, upd)
			case *sdcpb.LeafListSchema: // leaflist
				log.Debugf("TODO: handling leafList %s", item.Name)
			case string: // child container
				log.Debugf("handling child container %s", item)
				np := proto.Clone(p).(*sdcpb.Path)
				np.Elem = append(np.Elem, &sdcpb.PathElem{Name: item})
				rsp, err := d.schemaClient.GetSchema(ctx,
					&sdcpb.GetSchemaRequest{
						Path:   np,
						Schema: d.Schema().GetSchema(),
					})
				if err != nil {
					return nil, err
				}
				switch rsp := rsp.GetSchema().Schema.(type) {
				case *sdcpb.SchemaElem_Container:
					rs, err := d.expandContainerValue(ctx, np, v, rsp, includeKeysAsLeaf)
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
		upds := make([]*sdcpb.Update, 0)
		for _, v := range jv {
			np := proto.Clone(p).(*sdcpb.Path)
			r, err := d.expandContainerValue(ctx, np, v, cs, includeKeysAsLeaf)
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

func (d *Datastore) getItem(ctx context.Context, s string, cs *sdcpb.SchemaElem_Container) (any, bool) {
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

func isKey(s string, cs *sdcpb.SchemaElem_Container) bool {
	for _, k := range cs.Container.GetKeys() {
		if k.Name == s {
			return true
		}
	}
	return false
}

func getField(s string, cs *sdcpb.SchemaElem_Container) (*sdcpb.LeafSchema, bool) {
	for _, f := range cs.Container.GetFields() {
		if f.Name == s {
			return f, true
		}
	}
	return nil, false
}

func getLeafList(s string, cs *sdcpb.SchemaElem_Container) (*sdcpb.LeafListSchema, bool) {
	for _, lfl := range cs.Container.GetLeaflists() {
		if lfl.Name == s {
			return lfl, true
		}
	}
	return nil, false
}

func (d *Datastore) getChild(ctx context.Context, s string, cs *sdcpb.SchemaElem_Container) (string, bool) {
	for _, c := range cs.Container.GetChildren() {
		if c == s {
			return c, true
		}
	}
	if cs.Container.Name == "__root__" {
		for _, c := range cs.Container.GetChildren() {
			rsp, err := d.schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:   &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: c}}},
				Schema: d.Schema().GetSchema(),
			})
			if err != nil {
				log.Errorf("Failed to get schema object %s: %v", c, err)
				return "", false
			}
			switch rsp := rsp.GetSchema().Schema.(type) {
			case *sdcpb.SchemaElem_Container:
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

func (d *Datastore) doSubscribeOnce(ctx context.Context, subscription *sdcpb.Subscription, stream sdcpb.DataServer_SubscribeServer) error {
	paths := make([][]string, 0, len(subscription.GetPath()))
	for _, path := range subscription.GetPath() {
		paths = append(paths, utils.ToStrings(path, false, false))
	}

	for _, store := range getStores(subscription) {
		for upd := range d.cacheClient.ReadCh(ctx, d.config.Name, &cache.Opts{
			Store: store,
		}, paths, 0) {
			log.Debugf("ds=%s read path=%v from store=%v: %v", d.config.Name, paths, store, upd)
			rsp, err := d.subscribeResponseFromCacheUpdate(ctx, upd)
			if err != nil {
				return err
			}
			log.Debugf("ds=%s sending subscribe response: %v", d.config.Name, rsp)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				err = stream.Send(rsp)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func getStores(req proto.Message) []cachepb.Store {
	var dt sdcpb.DataType
	var candName string
	switch req := req.(type) {
	case *sdcpb.GetDataRequest:
		if req.GetDatastore().GetType() == sdcpb.Type_INTENDED {
			return []cachepb.Store{cachepb.Store_INTENDED}
		}
		dt = req.GetDataType()
		candName = req.GetDatastore().GetName()
	case *sdcpb.Subscription:
		dt = req.GetDataType()
	}

	var stores []cachepb.Store
	switch dt {
	case sdcpb.DataType_ALL:
		stores = []cachepb.Store{cachepb.Store_CONFIG}
		if candName == "" {
			stores = append(stores, cachepb.Store_STATE)
		}
	case sdcpb.DataType_CONFIG:
		stores = []cachepb.Store{cachepb.Store_CONFIG}
	case sdcpb.DataType_STATE:
		if candName == "" {
			stores = []cachepb.Store{cachepb.Store_STATE}
		}
	}
	return stores
}

func (d *Datastore) subscribeResponseFromCacheUpdate(ctx context.Context, upd *cache.Update) (*sdcpb.SubscribeResponse, error) {
	scp, err := d.toPath(ctx, upd.GetPath())
	if err != nil {
		return nil, err
	}
	tv, err := upd.Value()
	if err != nil {
		return nil, err
	}
	notification := &sdcpb.Notification{
		Timestamp: time.Now().UnixNano(),
		Update: []*sdcpb.Update{{
			Path:  scp,
			Value: tv,
		}},
	}
	return &sdcpb.SubscribeResponse{
		Response: &sdcpb.SubscribeResponse_Update{
			Update: notification,
		},
	}, nil
}

func (d *Datastore) setCandidate(ctx context.Context, req *sdcpb.SetDataRequest, expandDeletes bool) (*sdcpb.SetDataResponse, error) {
	ok, err := d.cacheClient.HasCandidate(ctx, req.GetName(), req.GetDatastore().GetName())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown candidate %s", req.GetDatastore().GetName())
	}

	replaces := make([]*sdcpb.Update, 0, len(req.GetReplace()))
	updates := make([]*sdcpb.Update, 0, len(req.GetUpdate()))

	// expand json/json_ietf values
	for _, upd := range req.GetReplace() {
		rs, err := d.expandUpdate(ctx, upd, false)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed expand replace: %v", err)
		}
		replaces = append(replaces, rs...)
	}

	for _, upd := range req.GetUpdate() {
		rs, err := d.expandUpdate(ctx, upd, false)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed expand update: %v", err)
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

	// validate individual deletes
	dels := make([]*sdcpb.Path, 0, len(req.GetDelete()))
	for _, del := range req.GetDelete() {
		if expandDeletes {
			rsp, err := d.schemaClient.ExpandPath(ctx, &sdcpb.ExpandPathRequest{
				Path:     del,
				Schema:   d.Schema().GetSchema(),
				DataType: sdcpb.DataType_CONFIG,
			})
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "delete path: %q validation failed: %v", del, err)
			}
			dels = append(dels, rsp.GetPath()...)
			continue
		}
		err = d.validatePath(ctx, del)
		if err != nil {
			return nil, err
		}
		dels = append(dels, del)
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
	rsp := &sdcpb.SetDataResponse{
		Response: make([]*sdcpb.UpdateResult, 0,
			len(dels)+len(replaces)+len(updates)),
	}

	name := fmt.Sprintf("%s/%s", req.GetName(), req.GetDatastore().GetName())
	cdels := make([][]string, 0, len(dels))
	upds := make([]*cache.Update, 0, len(replaces)+len(updates))
	// deletes start
	for _, del := range dels {
		cdels = append(cdels, utils.ToStrings(del, false, false))
	}
	for _, changes := range [][]*sdcpb.Update{replaces, updates} {
		for _, upd := range changes {
			cUpd, err := d.cacheClient.NewUpdate(upd)
			if err != nil {
				return nil, err
			}
			upds = append(upds, cUpd)
		}
	}
	err = d.cacheClient.Modify(ctx, name, &cache.Opts{
		Store: cachepb.Store_CONFIG,
	}, cdels, upds)
	if err != nil {
		return nil, err
	}

	// validate Choice Cases
	log.Infof("%s: validating choice cases candidate %s", d.Name(), req.GetDatastore().GetName())
	if err = d.validateChoiceCases(ctx, updates, replaces, req.Datastore.Name); err != nil {
		return nil, err
	}

	// deletes start
	for _, del := range req.GetDelete() {
		rsp.Response = append(rsp.Response, &sdcpb.UpdateResult{
			Path: del,
			Op:   sdcpb.UpdateResult_DELETE,
		})
	}
	// deletes end
	// replaces start
	for _, rep := range req.GetReplace() {
		rsp.Response = append(rsp.Response, &sdcpb.UpdateResult{
			Path: rep.GetPath(),
			Op:   sdcpb.UpdateResult_REPLACE,
		})
	}
	// replaces end
	// updates start
	for _, upd := range req.GetUpdate() {
		rsp.Response = append(rsp.Response, &sdcpb.UpdateResult{
			Path: upd.GetPath(),
			Op:   sdcpb.UpdateResult_UPDATE,
		})
	}
	// updates end
	return rsp, nil
}

func (d *Datastore) convertTypedValueToProto(ctx context.Context, p *sdcpb.Path, tv *sdcpb.TypedValue) (*sdcpb.TypedValue, error) {
	rsp, err := d.getSchema(ctx, p)
	if err != nil {
		return nil, err
	}
	return convertTypedValueToYANGType(rsp.GetSchema(), tv)
}

func convertTypedValueToYANGType(schemaElem *sdcpb.SchemaElem, tv *sdcpb.TypedValue) (*sdcpb.TypedValue, error) {
	switch {
	case schemaElem.GetContainer() != nil:
		if schemaElem.GetContainer().IsPresence {
			return &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_JsonVal{JsonVal: nil},
			}, nil
		}
	case schemaElem.GetLeaflist() != nil:
		switch tv.Value.(type) {
		case *sdcpb.TypedValue_LeaflistVal:
			return tv, nil
		}
		return &sdcpb.TypedValue{
			Timestamp: tv.GetTimestamp(),
			Value: &sdcpb.TypedValue_LeaflistVal{
				LeaflistVal: &sdcpb.ScalarArray{
					Element: []*sdcpb.TypedValue{tv},
				},
			},
		}, nil
	case schemaElem.GetField() != nil:
		switch schemaElem.GetField().GetType().GetType() {
		default:
			return tv, nil
		case "string", "identityref":
			return tv, nil
		case "uint64", "uint32", "uint16", "uint8":
			i, err := strconv.Atoi(utils.TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			ctv := &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_UintVal{UintVal: uint64(i)},
			}
			return ctv, nil
		case "int64", "int32", "int16", "int8":
			i, err := strconv.Atoi(utils.TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			ctv := &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_IntVal{IntVal: int64(i)},
			}
			return ctv, nil
		case "enumeration":
			return tv, nil
		case "union":
			return tv, nil
		case "boolean":
			v, err := strconv.ParseBool(utils.TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: v}}, nil
		case "decimal64":
			d64, err := utils.ParseDecimal64(utils.TypedValueToString(tv))
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{
				Value: &sdcpb.TypedValue_DecimalVal{
					DecimalVal: d64,
				},
			}, nil
		case "float":
			v, err := strconv.ParseFloat(utils.TypedValueToString(tv), 32)
			if err != nil {
				return nil, err
			}
			return &sdcpb.TypedValue{
				Timestamp: tv.GetTimestamp(),
				Value:     &sdcpb.TypedValue_FloatVal{FloatVal: float32(v)},
			}, nil
		}
	}
	return nil, nil
}
