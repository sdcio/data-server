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
	"runtime"
	"strings"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/config"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/pool"
	"github.com/sdcio/data-server/pkg/tree"
	jsonImporter "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

var (
	// TypedValue Bool True and false
	TypedValueTrue   = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: true}}
	TypedValueFalse  = &sdcpb.TypedValue{Value: &sdcpb.TypedValue_BoolVal{BoolVal: false}}
	validationConfig = config.NewValidationConfig()
)

func init() {
	validationConfig.SetDisableConcurrency(true)
}

func TestDatastore_populateTree(t *testing.T) {
	prio15 := int32(15)
	prio10 := int32(10)
	prio5 := int32(5)
	owner1 := "owner1"
	owner2 := "owner2"
	owner3 := "owner3"

	_ = prio15
	_ = prio10
	_ = prio5
	_ = owner1
	_ = owner2
	_ = owner3

	tests := []struct {
		name                 string
		intentReqValue       func() (string, error) // depending on the path, this should be *testhelper.TestConfig or any sub-value
		intentReqPath        *sdcpb.Path
		intentName           string
		intentPrio           int32
		intentDelete         bool
		expectedModify       []*types.Update
		expectedDeletes      []string // xpath deletes
		expectedOwnerDeletes []string // xpath deletes
		expectedOwnerUpdates []*types.Update
		intendedStoreUpdates []*types.Update
		runningStoreUpdates  []*types.Update
		NotOnlyNewOrUpdated  bool // it negated when used in the call, usually we want it to be true
	}{
		{
			name:          "DoubleKey - Delete Single item",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
						{
							Key1: "k1.1",
							Key2: "k1.2",
						}: {
							Key1:    ygot.String("k1.1"),
							Key2:    ygot.String("k1.2"),
							Mandato: ygot.String("TheMandatoryValueOther"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
						{
							Key1: "k2.1",
							Key2: "k2.2",
						}: {
							Key1:    ygot.String("k2.1"),
							Key2:    ygot.String("k2.2"),
							Mandato: ygot.String("TheMandatoryValue2"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval2.1"),
								Value2: ygot.String("containerval2.2"),
							},
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.3"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},

			expectedDeletes: []string{
				"/doublekey[key1=k1.1][key2=k1.3]",
			},

			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValueOther"), prio10, owner2, 0),
			},

			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValueOther"), prio10, owner2, 0),
			},
			expectedOwnerDeletes: []string{
				"/doublekey[key1=k1.1][key2=k1.3]/key1",
				"/doublekey[key1=k1.1][key2=k1.3]/key2",
				"/doublekey[key1=k1.1][key2=k1.3]/mandato",
				"/doublekey[key1=k1.1][key2=k1.3]/cont/value1",
				"/doublekey[key1=k1.1][key2=k1.3]/cont/value2",
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.3"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.2"), prio10, owner2, 0),
			},
		},
		{
			name:          "DoubleKey - New Data",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Doublekey: map[sdcio_schema.SdcioModel_Doublekey_Key]*sdcio_schema.SdcioModel_Doublekey{
						{
							Key1: "k1.1",
							Key2: "k1.2",
						}: {
							Key1:    ygot.String("k1.1"),
							Key2:    ygot.String("k1.2"),
							Mandato: ygot.String("TheMandatoryValue1"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
						{
							Key1: "k2.1",
							Key2: "k2.2",
						}: {
							Key1:    ygot.String("k2.1"),
							Key2:    ygot.String("k2.2"),
							Mandato: ygot.String("TheMandatoryValue2"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval2.1"),
								Value2: ygot.String("containerval2.2"),
							},
						},
						{
							Key1: "k1.1",
							Key2: "k1.3",
						}: {
							Key1:    ygot.String("k1.1"),
							Key2:    ygot.String("k1.3"),
							Mandato: ygot.String("TheMandatoryValue1"),
							Cont: &sdcio_schema.SdcioModel_Doublekey_Cont{
								Value1: ygot.String("containerval1.1"),
								Value2: ygot.String("containerval1.2"),
							},
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.3"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.2"), prio10, owner2, 0),
			},

			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k1.3"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k1.1", "key2": "k1.3"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval1.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("key2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("k2.2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("mandato", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("TheMandatoryValue2"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value1", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.1"), prio10, owner2, 0),

				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("doublekey", map[string]string{"key1": "k2.1", "key2": "k2.2"}),
					sdcpb.NewPathElem("cont", nil),
					sdcpb.NewPathElem("value2", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("containerval2.2"), prio10, owner2, 0),
			},

			intendedStoreUpdates: []*types.Update{},
		},
		{
			name:          "Simple add to root path",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{},
				}
				d.Interface["ethernet-1/1"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyDescription"),
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},

			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: nil,
		},
		{
			name:          "Simple add with a specific path",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", nil)}},

			intentReqValue: func() (string, error) {
				i := &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyDescription"),
				}
				return ygot.EmitJSON(i, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: nil,
		},
		{
			name:          "Add with existing better prio same intent name",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", nil)}},
			intentReqValue: func() (string, error) {
				i := &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyDescription"),
				}
				return ygot.EmitJSON(i, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				// Right now, although the value stays the same, but the priority changes, we'll receive an update for these values.
				// This maybe needs to be mitigated, but is not considered harmfull atm.
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
			},
		},
		{
			name:          "Delete the highes priority values, making shadowed values become active",
			intentName:    owner1,
			intentPrio:    prio5,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				return "{}", nil
			},
			intentDelete: true,
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
			},
			expectedDeletes: []string{
				"/interface[name=ethernet-1/1]/name",
				"/interface[name=ethernet-1/1]/description",
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
			},
		},
		{
			name:          "Delete - aggregate branch via keys",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				return "{}", nil
			},
			intentDelete: true,
			expectedDeletes: []string{
				"/interface[name=ethernet-1/1]/",
			},
			expectedOwnerDeletes: []string{
				"/interface[name=ethernet-1/1]/name",
				"/interface[name=ethernet-1/1]/description",
				"/interface[name=ethernet-1/1]/admin-state",
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("admin-state", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
			},
		},
		{
			name:          "Delete - aggregate branch via keys, multiple entries (different keys)",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:        ygot.String("ethernet-1/1"),
							Description: ygot.String("MyNonappliedDescription"),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			intentDelete:        true,
			runningStoreUpdates: []*types.Update{
				// cache.NewUpdate([]string{"interface", "ethernet-1/1", "name"}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/1", "description"}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/1", "admin-state"}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/2", "name"}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/2", "description"}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/2", "admin-state"}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/3", "name"}, testhelper.GetStringTvProto("ethernet-1/3"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/3", "description"}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-1/3", "admin-state"}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
			},
			expectedDeletes: []string{
				"/interface[name=ethernet-1/1]/admin-state",
				"/interface[name=ethernet-1/2]",
				"/interface[name=ethernet-1/3]",
			},
			expectedOwnerDeletes: []string{
				"/interface[name=ethernet-1/1]/admin-state",
				"/interface[name=ethernet-1/2]/name",
				"/interface[name=ethernet-1/2]/description",
				"/interface[name=ethernet-1/2]/admin-state",
				"/interface[name=ethernet-1/3]/name",
				"/interface[name=ethernet-1/3]/description",
				"/interface[name=ethernet-1/3]/admin-state",
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyNonappliedDescription"), prio10, owner2, 0),
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyNonappliedDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("admin-state", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}), sdcpb.NewPathElem("admin-state", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/3"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/3"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/3"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescriptionOwner2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/3"}), sdcpb.NewPathElem("admin-state", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("enable"), prio10, owner2, 0),
			},
		},
		{
			name:          "Add lower precedence intent, every value already shadowed",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", nil)}},
			intentReqValue: func() (string, error) {
				i := &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyNonappliedDescription"),
				}
				return ygot.EmitJSON(i, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyNonappliedDescription"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
			},
		},
		{
			name:          "choices delete",
			intentReqPath: &sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", nil)}},
			intentReqValue: func() (string, error) {
				i := &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyDescription"),
				}
				return ygot.EmitJSON(i, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			intentPrio: 10,
			intentName: owner1,
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("choices", nil), sdcpb.NewPathElem("case1", nil), sdcpb.NewPathElem("case-elem", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("Foobar"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("choices", nil), sdcpb.NewPathElem("case1", nil), sdcpb.NewPathElem("case-elem", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("Foobar"), prio10, owner1, 0),
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("name", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}), sdcpb.NewPathElem("description", nil)}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio10, owner1, 0),
			},
			expectedDeletes: []string{
				"/choices",
			},
			expectedOwnerDeletes: []string{
				"/choices/case1/case-elem",
			},
		},
		{
			name:          "Mixed, new entry, higher and lower precedence",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,

			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{},
				}
				d.Interface["ethernet-1/1"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyOtherDescription"),
					Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
						1: {
							Index:       ygot.Uint32(1),
							Description: ygot.String("Subinterface Desc"),
						},
					},
				}

				d.Interface["ethernet-1/2"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/2"),
					Description: ygot.String("MyOtherDescription"),
					Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
						1: {
							Index:       ygot.Uint32(1),
							Description: ygot.String("Subinterface Desc"),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Owner3 Description"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(2), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Owner3 Description"), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(2), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio15, owner3, 0),
			},
		},
		{
			name:                "Mixed, new entry, higher and lower precedence. notOnlyUpdated set to TRUE",
			intentName:          owner2,
			intentPrio:          prio10,
			intentReqPath:       nil,
			NotOnlyNewOrUpdated: true,

			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{},
				}
				d.Interface["ethernet-1/1"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/1"),
					Description: ygot.String("MyOtherDescription"),
					Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
						1: {
							Index:       ygot.Uint32(1),
							Description: ygot.String("Subinterface Desc"),
						},
					},
				}

				d.Interface["ethernet-1/2"] = &sdcio_schema.SdcioModel_Interface{
					Name:        ygot.String("ethernet-1/2"),
					Description: ygot.String("MyOtherDescription"),
					Subinterface: map[uint32]*sdcio_schema.SdcioModel_Interface_Subinterface{
						1: {
							Index:       ygot.Uint32(1),
							Description: ygot.String("Subinterface Desc"),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Owner3 Description"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio15, owner3, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyOtherDescription"), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio10, owner2, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/1"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/1"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("MyDescription"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("name", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("ethernet-1/2"), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Owner3 Description"), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("index", nil),
				}, IsRootBased: true}, testhelper.GetUIntTvProto(1), prio15, owner3, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("interface", map[string]string{"name": "ethernet-1/2"}),
					sdcpb.NewPathElem("subinterface", map[string]string{"index": "2"}),
					sdcpb.NewPathElem("description", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("Subinterface Desc"), prio15, owner3, 0),
			},
		},
		{
			name:          "ChoiceCase - new highest case",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Choices: &sdcio_schema.SdcioModel_Choices{
						Case2: &sdcio_schema.SdcioModel_Choices_Case2{
							Log: ygot.Bool(true),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case2", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueTrue, prio10, owner2, 0),
			},
			expectedDeletes: []string{
				"/choices/case1",
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case2", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueTrue, prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("case-elem", nil),
					sdcpb.NewPathElem("elem", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("case1-content"), prio15, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueFalse, prio15, owner1, 0),
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("case-elem", nil),
					sdcpb.NewPathElem("elem", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("case1-content"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueFalse, tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},
		},
		{
			name:          "ChoiceCase - old highest case",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Choices: &sdcio_schema.SdcioModel_Choices{
						Case2: &sdcio_schema.SdcioModel_Choices_Case2{
							Log: ygot.Bool(true),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				// no mods expected
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case2", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueTrue, prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("case-elem", nil),
					sdcpb.NewPathElem("elem", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("case1-content"), prio5, owner1, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueFalse, prio5, owner1, 0),
			},
			runningStoreUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("case-elem", nil),
					sdcpb.NewPathElem("elem", nil),
				}, IsRootBased: true}, testhelper.GetStringTvProto("case1-content"), tree.RunningValuesPrio, tree.RunningIntentName, 0),
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case1", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueFalse, tree.RunningValuesPrio, tree.RunningIntentName, 0),
			},
		},
		{
			name:          "ChoiceCase - add first case",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: nil,
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Choices: &sdcio_schema.SdcioModel_Choices{
						Case2: &sdcio_schema.SdcioModel_Choices_Case2{
							Log: ygot.Bool(true),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			expectedModify: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case2", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueTrue, prio10, owner2, 0),
			},
			expectedOwnerUpdates: []*types.Update{
				types.NewUpdate(&sdcpb.Path{Elem: []*sdcpb.PathElem{
					sdcpb.NewPathElem("choices", nil),
					sdcpb.NewPathElem("case2", nil),
					sdcpb.NewPathElem("log", nil),
				}, IsRootBased: true}, TypedValueTrue, prio10, owner2, 0),
			},
			intendedStoreUpdates: []*types.Update{},
			runningStoreUpdates:  []*types.Update{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create a gomock controller
			controller := gomock.NewController(t)
			defer controller.Finish()

			ctx := context.Background()

			sc, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Fatal(err)
			}
			scb := schemaClient.NewSchemaClientBound(schema, sc)
			tc := tree.NewTreeContext(scb, tt.intentName)

			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Error(err)
			}

			jconfStr, err := tt.intentReqValue()
			if err != nil {
				t.Error(err)
			}

			var jsonConfAny any
			err = json.Unmarshal([]byte(jconfStr), &jsonConfAny)
			if err != nil {
				t.Error(err)
			}

			// add intended content
			err = root.AddUpdatesRecursive(ctx, tt.intendedStoreUpdates, types.NewUpdateInsertFlags())
			if err != nil {
				t.Error(err)
			}

			// add running content
			err = root.AddUpdatesRecursive(ctx, tt.runningStoreUpdates, types.NewUpdateInsertFlags())
			if err != nil {
				t.Error(err)
			}

			sharedTaskPool := pool.NewSharedTaskPool(ctx, runtime.NumCPU())
			deleteVisitorPool := sharedTaskPool.NewVirtualPool(pool.VirtualFailFast, 1)
			ownerDeleteMarker := tree.NewOwnerDeleteMarker(tree.NewOwnerDeleteMarkerTaskConfig(tt.intentName, false))

			err = ownerDeleteMarker.Run(root.GetRoot(), deleteVisitorPool)
			if err != nil {
				t.Error(err)
				return
			}

			// marksOwnerDeleteVisitor := tree.NewMarkOwnerDeleteVisitor(tt.intentName, false)
			// err = root.Walk(ctx, marksOwnerDeleteVisitor)
			// if err != nil {
			// 	t.Error(err)
			// }

			newFlag := types.NewUpdateInsertFlags().SetNewFlag()

			err = root.ImportConfig(ctx, tt.intentReqPath, jsonImporter.NewJsonTreeImporter(jsonConfAny), tt.intentName, tt.intentPrio, newFlag)
			if err != nil {
				t.Error(err)
			}

			fmt.Println(root.String())
			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}
			fmt.Println(root.String())

			validationResult, _ := root.Validate(ctx, validationConfig)

			fmt.Printf("Validation Errors:\n%v\n", strings.Join(validationResult.ErrorsStr(), "\n"))
			fmt.Printf("Tree:%s\n", root.String())

			// get the updates that are meant to be send down towards the device
			updates := root.GetHighestPrecedence(!tt.NotOnlyNewOrUpdated)
			if diff := testhelper.DiffUpdates(tt.expectedModify, updates.ToUpdateSlice()); diff != "" {
				t.Errorf("root.GetHighestPrecedence(%t) mismatch (-want +got):\n%s", !tt.NotOnlyNewOrUpdated, diff)
			}

			// get the deletes that are meant to be send down towards the device
			deletes, err := root.GetDeletes(true)
			if err != nil {
				t.Error(err)
			}

			deletePathSlice := make(sdcpb.Paths, 0, len(deletes))
			for _, del := range deletes {
				deletePathSlice = append(deletePathSlice, del.SdcpbPath())
			}

			if diff := testhelper.DiffStringSlice(tt.expectedDeletes, deletePathSlice.ToXPathSlice(), true); diff != "" {
				t.Errorf("root.GetDeletes() mismatch (-want +got):\n%s", diff)
			}

			// get the updates that are meant to be send down towards the cache (INTENDED)
			updatesOwner := root.GetUpdatesForOwner(tt.intentName)
			if diff := testhelper.DiffUpdates(tt.expectedOwnerUpdates, updatesOwner); diff != "" {
				t.Errorf("root.GetUpdatesForOwner mismatch (-want +got):\n%s", diff)
			}

			// get the deletes that are meant to be send down towards the cache (INTENDED)
			deletesOwner := root.GetDeletesForOwner(tt.intentName)
			if diff := testhelper.DiffStringSlice(tt.expectedOwnerDeletes, deletesOwner.ToXPathSlice(), true); diff != "" {
				t.Errorf("root.GetDeletesForOwner mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
