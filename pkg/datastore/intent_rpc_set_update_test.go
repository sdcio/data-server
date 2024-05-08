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
	"testing"

	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/mocks/mockschema"
	"github.com/sdcio/data-server/mocks/mocktarget"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/config"
	"github.com/sdcio/data-server/pkg/utils"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
)

func TestDatastore_populateTree(t *testing.T) {
	prio15 := int32(15)
	prio10 := int32(10)
	prio5 := int32(5)
	owner1 := "owner1"
	owner2 := "owner2"
	owner3 := "owner3"

	tests := []struct {
		name                 string
		intentReqValue       interface{} // depending on the path, this should be *testhelper.TestConfig or any sub-value
		intentReqPath        string
		intentName           string
		intentPrio           int32
		intentDelete         bool
		expectedModify       []*cache.Update
		expectedDeletes      [][]string
		expectedOwnerDeletes [][]string
		expectedOwnerUpdates []*cache.Update
		intendedStoreUpdates []*cache.Update
		NotOnlyNewOrUpdated  bool // it negated when used in the call, usually we want it to be true
	}{
		{
			name:          "Simple add to root path",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: "/",
			intentReqValue: &testhelper.TestConfig{
				Interf: []*testhelper.Interf{
					{
						Name:        "ethernet-0/0",
						Description: "MyDescription",
					},
				},
			},
			expectedModify: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: nil,
		},
		{
			name:          "Simple add with a specific path",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: "interface",
			intentReqValue: &testhelper.Interf{
				Name:        "ethernet-0/0",
				Description: "MyDescription",
			},
			expectedModify: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: nil,
		},
		{
			name:          "Add with existing better prio same intent / owner",
			intentName:    owner1,
			intentPrio:    prio10,
			intentReqPath: "interface",
			intentReqValue: &testhelper.Interf{
				Name:        "ethernet-0/0",
				Description: "MyDescription",
			},
			expectedModify: []*cache.Update{
				// Right now, although the value stays the same, but the priority changes, we'll receive an update for these values.
				// This maybe needs to be mitigated, but is not considered harmfull atm.
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio10, owner1, 0),
			},
			intendedStoreUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
			},
		},
		{
			name:           "Delete the highes priority values, making shadowed values become active",
			intentName:     owner1,
			intentPrio:     prio10,
			intentReqPath:  "/",
			intentReqValue: struct{}{},
			intentDelete:   true,
			expectedModify: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescriptionOwner2"), prio10, owner2, 0),
			},
			expectedOwnerDeletes: [][]string{
				{"interface", "ethernet-0/0", "name"},
				{"interface", "ethernet-0/0", "description"},
			},
			intendedStoreUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescriptionOwner2"), prio10, owner2, 0),
			},
		},
		{
			name:          "Add lower precedence intent, every value already shadowed",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: "interface",
			intentReqValue: &testhelper.Interf{
				Name:        "ethernet-0/0",
				Description: "MyNonappliedDescription",
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyNonappliedDescription"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
			},
		},
		{
			name:          "Mixed, new entry, higher and lower precedence",
			intentName:    owner2,
			intentPrio:    prio10,
			intentReqPath: "/",
			intentReqValue: &testhelper.TestConfig{
				Interf: []*testhelper.Interf{
					{
						Name:        "ethernet-0/0",
						Description: "MyOtherDescription",
						Subinterface: []*testhelper.Subinterface{
							{
								Index:       1,
								Description: "Subinterface Desc",
							},
						},
					},
					{
						Name:        "ethernet-0/1",
						Description: "MyOtherDescription",
						Subinterface: []*testhelper.Subinterface{
							{
								Index:       1,
								Description: "Subinterface Desc",
							},
						},
					},
				},
			},
			expectedModify: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "Owner3 Description"), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "index"}, testhelper.GetUIntTvProto(t, 1), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio15, owner3, 0),
			},
		},
		{
			name:                "Mixed, new entry, higher and lower precedence. notOnlyUpdated set to TRUE",
			intentName:          owner2,
			intentPrio:          prio10,
			intentReqPath:       "/",
			NotOnlyNewOrUpdated: true,
			intentReqValue: &testhelper.TestConfig{
				Interf: []*testhelper.Interf{
					{
						Name:        "ethernet-0/0",
						Description: "MyOtherDescription",
						Subinterface: []*testhelper.Subinterface{
							{
								Index:       1,
								Description: "Subinterface Desc",
							},
						},
					},
					{
						Name:        "ethernet-0/1",
						Description: "MyOtherDescription",
						Subinterface: []*testhelper.Subinterface{
							{
								Index:       1,
								Description: "Subinterface Desc",
							},
						},
					},
				},
			},
			expectedModify: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
				// the next two are not part of the result, we might want to add the behaviour, such that one can query the entire intended.
				// cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "index"}, testhelper.GetUIntTvProto(t, 1), prio15, owner3, 0),
				// cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio15, owner3, 0),
			},
			expectedOwnerUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "MyOtherDescription"), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "index"}, testhelper.GetUIntTvProto(t, 1), prio10, owner2, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "1", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio10, owner2, 0),
			},
			intendedStoreUpdates: []*cache.Update{
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, testhelper.GetStringTvProto(t, "MyDescription"), prio5, owner1, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, testhelper.GetStringTvProto(t, "Owner3 Description"), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "index"}, testhelper.GetUIntTvProto(t, 1), prio15, owner3, 0),
				cache.NewUpdate([]string{"interface", "ethernet-0/1", "subinterface", "2", "description"}, testhelper.GetStringTvProto(t, "Subinterface Desc"), prio15, owner3, 0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create a gomock controller
			controller := gomock.NewController(t)

			// create a cache client mock
			cacheClient := mockcacheclient.NewMockClient(controller)
			testhelper.ConfigureCacheClientMock(t, cacheClient, tt.intendedStoreUpdates, tt.expectedModify, tt.expectedDeletes)

			// create a schema client mock
			schemaClient := mockschema.NewMockClient(controller)
			testhelper.ConfigureSchemaClientMock(t, schemaClient)

			dsName := "dev1"

			// create a datastore
			d := &Datastore{
				config: &config.DatastoreConfig{
					Name: dsName,
					Schema: &config.SchemaConfig{
						Name:    "test",
						Vendor:  "test",
						Version: "v0.0.0",
					},
				},

				sbi:          mocktarget.NewMockTarget(controller),
				cacheClient:  cacheClient,
				schemaClient: schemaClient,
			}

			ctx := context.Background()

			// marshall the intentReqValue into a byte slice
			byteConfig, err := json.Marshal(tt.intentReqValue)
			if err != nil {
				t.Error(err)
			}

			// parse the path under which the intent value is to be put
			path, err := utils.ParsePath(tt.intentReqPath)
			if err != nil {
				t.Error(err)
			}

			// prepare the SetintentRequest
			reqOne := &sdcpb.SetIntentRequest{
				Name:     dsName,
				Intent:   tt.intentName,
				Priority: tt.intentPrio,
				Update: []*sdcpb.Update{
					{
						Path: path,
						Value: &sdcpb.TypedValue{
							Value: &sdcpb.TypedValue_JsonVal{
								JsonVal: byteConfig},
						},
					},
				},
				Delete: tt.intentDelete,
			}

			// Populate the root tree
			root, err := d.populateTree(ctx, reqOne)
			if err != nil {
				t.Error(err)
			}

			// get the updates that are meant to be send down towards the device
			updates := root.GetHighesPrio(!tt.NotOnlyNewOrUpdated)
			if diff := testhelper.DiffCacheUpdates(tt.expectedModify, updates); diff != "" {
				t.Errorf("root.GetHighesPrio(true) mismatch (-want +got):\n%s", diff)
			}

			// get the deletes that are meant to be send down towards the device
			deletes := root.GetDeletes()
			if diff := testhelper.DiffDoubleStringPathSlice(tt.expectedDeletes, deletes); diff != "" {
				t.Errorf("root.GetDeletes() mismatch (-want +got):\n%s", diff)
			}

			// get the updates that are meant to be send down towards the cache (INTENDED)
			updatesOwner := root.GetUpdatesForOwner(tt.intentName)
			if diff := testhelper.DiffCacheUpdates(tt.expectedOwnerUpdates, updatesOwner); diff != "" {
				t.Errorf("root.GetUpdatesForOwner mismatch (-want +got):\n%s", diff)
			}

			// get the deletes that are meant to be send down towards the cache (INTENDED)
			deletesOwner := root.GetDeletesForOwner(tt.intentName)
			if diff := testhelper.DiffDoubleStringPathSlice(tt.expectedOwnerDeletes, deletesOwner); diff != "" {
				t.Errorf("root.GetDeletesForOwner mismatch (-want +got):\n%s", diff)
			}
		})
	}

}
