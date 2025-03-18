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
	"slices"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/pkg/cache"
	schemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/tree"
	json_importer "github.com/sdcio/data-server/pkg/tree/importer/json"
	"github.com/sdcio/data-server/pkg/tree/types"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
)

func TestDatastore_validateTree(t *testing.T) {
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
		intentReqPath        string
		intentName           string
		intentPrio           int32
		intentDelete         bool
		intendedStoreUpdates []*cache.Update
		NotOnlyNewOrUpdated  bool // it negated when used in the call, usually we want it to be true
		expectedWarnings     []string
	}{

		{
			name:          "deref",
			intentReqPath: "/",
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:          ygot.String("ethernet-1/1"),
							InterfaceType: ygot.String("traffic"),
							AdminState:    sdcio_schema.SdcioModelIf_AdminState_enable,
						},
						"mgmt0": {
							Name:          ygot.String("mgmt0"),
							InterfaceType: ygot.String("mgmt"),
							AdminState:    sdcio_schema.SdcioModelIf_AdminState_enable,
						},
					},
					MgmtInterface: &sdcio_schema.SdcioModel_MgmtInterface{
						Name: ygot.String("mgmt0"),
						Type: ygot.String("mgmt"),
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			intentName: owner1,
			intentPrio: prio10,
		},
		{
			name:          "identityref",
			intentReqPath: "/",
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"ethernet-1/1": {
							Name:          ygot.String("ethernet-1/1"),
							InterfaceType: ygot.String("traffic"),
							AdminState:    sdcio_schema.SdcioModelIf_AdminState_enable,
						},
						"mgmt0": {
							Name:          ygot.String("mgmt0"),
							InterfaceType: ygot.String("mgmt"),
							AdminState:    sdcio_schema.SdcioModelIf_AdminState_enable,
						},
					},
					MgmtInterface: &sdcio_schema.SdcioModel_MgmtInterface{
						Name: ygot.String("mgmt0"),
						Type: ygot.String("mgmt"),
					},
					Identityref: &sdcio_schema.SdcioModel_Identityref{
						CryptoA: sdcio_schema.SdcioModelIdentityBase_CryptoAlg_des3,
						CryptoB: sdcio_schema.SdcioModelIdentityBase_CryptoAlg_otherAlgo,
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			intentName: owner1,
			intentPrio: prio10,
		},
		{
			name:          "leafref-optional (require-instance == false) not exists",
			intentReqPath: "/",
			intentReqValue: func() (string, error) {
				return "{\"leafref-optional\":\"mgmt0\"}", nil
			},
			intentName:       owner1,
			intentPrio:       prio10,
			expectedWarnings: []string{"leafref leafref-optional value mgmt0 unable to resolve non-mandatory reference /interface/name"},
		},
		{
			name:          "leafref-optional (require-instance == false) exists",
			intentReqPath: "/",
			intentReqValue: func() (string, error) {
				d := &sdcio_schema.Device{
					LeafrefOptional: ygot.String("mgmt0"),
					Interface: map[string]*sdcio_schema.SdcioModel_Interface{
						"mgmt0": {
							Name:        ygot.String("mgmt0"),
							Description: ygot.String("foo"),
						},
					},
				}
				return ygot.EmitJSON(d, &ygot.EmitJSONConfig{
					Format:         ygot.RFC7951,
					SkipValidation: false,
				})
			},
			intentName: owner1,
			intentPrio: prio10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			sc, schema, err := testhelper.InitSDCIOSchema()
			if err != nil {
				t.Error(err)
			}
			scb := schemaClient.NewSchemaClientBound(schema, sc)
			ctx := context.Background()

			// marshall the intentReqValue into a byte slice
			jsonConfString, err := tt.intentReqValue()
			if err != nil {
				t.Error(err)
			}

			var jsonConf any
			err = json.Unmarshal([]byte(jsonConfString), &jsonConf)
			if err != nil {
				t.Error(err)
			}

			// parse the path under which the intent value is to be put
			path, err := utils.ParsePath(tt.intentReqPath)
			if err != nil {
				t.Error(err)
			}

			tc := tree.NewTreeContext(scb, tt.intentName)
			root, err := tree.NewTreeRoot(ctx, tc)
			if err != nil {
				t.Error(err)
			}

			flagsNew := types.NewUpdateInsertFlags()
			flagsNew.SetNewFlag()

			importer := json_importer.NewJsonTreeImporter(jsonConf)

			err = root.ImportConfig(ctx, utils.ToStrings(path, false, false), importer, tt.intentName, tt.intentPrio, flagsNew)
			if err != nil {
				t.Error(err)
			}

			err = root.FinishInsertionPhase(ctx)
			if err != nil {
				t.Error(err)
			}

			validationResult := root.Validate(ctx, false)

			t.Log(root.String())

			for _, x := range tt.expectedWarnings {
				if !slices.Contains(validationResult.WarningsStr(), x) {
					t.Errorf("Warning %q not emitted.", x)
				}
			}

			fmt.Println(validationResult.ErrorsStr())
			fmt.Println(validationResult.WarningsStr())

			fmt.Printf("Tree:%s\n", root.String())
		})
	}
}
