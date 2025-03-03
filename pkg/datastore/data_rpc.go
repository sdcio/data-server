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
	"fmt"
	"math"
	"strconv"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/sdcio/data-server/pkg/utils"
)

const (
	// to be used for candidates created without an owner
	DefaultOwner = "__sdcio"
)

func (d *Datastore) validateUpdate(ctx context.Context, upd *sdcpb.Update) error {
	// 1.validate the path i.e check that the path exists
	// 2.validate that the value is compliant with the schema

	// 1. validate the path
	rsp, err := d.schemaClient.GetSchemaSdcpbPath(ctx, upd.GetPath())
	if err != nil {
		return err
	}
	// 2. convert value to its YANG type
	upd.Value, err = utils.ConvertTypedValueToYANGType(rsp.GetSchema(), upd.GetValue())
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
		if !pathIsKeyAsLeaf(upd.GetPath()) && !obj.Container.IsPresence {
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
		case int64:
			// No need to do anything, same type
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
		case uint64:
			// No need to do anything, same type
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
		for _, vv := range lt.EnumNames {
			if fmt.Sprintf("%s", v) == vv {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %q does not match enum type %q, must be one of [%s]", v, lt.TypeName, strings.Join(lt.EnumNames, ", "))
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
		identities := make([]string, 0, len(lt.IdentityPrefixesMap))
		for vv := range lt.IdentityPrefixesMap {
			identities = append(identities, vv)
			if fmt.Sprintf("%s", v) == vv {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %q does not match identityRef type %q, must be one of [%s]", v, lt.TypeName, strings.Join(identities, ", "))
		}
		return nil
	case "decimal64":
		switch v := v.(type) {
		case float64: // if it's a float64 then it's a valid decimal64
		case string:
			if c := strings.Count(v, "."); c == 0 || c > 1 {
				return fmt.Errorf("value %q is not a valid Decimal64", v)
			}
		case sdcpb.Decimal64, *sdcpb.Decimal64:
			// No need to do anything, same type
		default:
			return fmt.Errorf("unexpected type for a Decimal64 value %q: %T", v, v)
		}
		return nil
	case "leafref":
		// TODO: does this need extra validation?
		return nil
	case "empty":
		switch v.(type) {
		case *emptypb.Empty:
			return nil
		}
		return fmt.Errorf("value %v is not an empty JSON object '{}' so does not match empty type", v)
	default:
		return fmt.Errorf("unhandled type %v for value %q", lt.GetType(), v)
	}
}

func validateLeafListValue(ll *sdcpb.LeafListSchema, v any) error {
	switch vTyped := v.(type) {
	case *sdcpb.ScalarArray:
		for _, elem := range vTyped.Element {
			val, err := utils.GetSchemaValue(elem)
			if err != nil {
				return err
			}
			err = validateLeafTypeValue(ll.GetType(), val)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
