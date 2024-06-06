package tree

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/ygot/ygot"
	"github.com/sdcio/data-server/mocks/mockschemaclientbound"
	"github.com/sdcio/data-server/pkg/cache"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils/testhelper"
	sdcio_schema "github.com/sdcio/data-server/tests/sdcioygot"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
)

func Test_Entry(t *testing.T) {

	desc, err := proto.Marshal(&sdcpb.TypedValue{Value: &sdcpb.TypedValue_StringVal{StringVal: "MyDescription"}})
	if err != nil {
		t.Error(err)
	}

	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "9", "description"}, desc, int32(100), "me", int64(9999999))
	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc, int32(99), "me", int64(444))
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc, int32(98), "me", int64(88))

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	for _, u := range []*cache.Update{u1, u2, u3} {
		err = root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Error(err)
		}
	}

	r := []string{}
	r = root.StringIndent(r)
	t.Log(strings.Join(r, "\n"))
}

func Test_Entry_One(t *testing.T) {
	desc1 := testhelper.GetStringTvProto(t, "DescriptionOne")
	desc2 := testhelper.GetStringTvProto(t, "DescriptionTwo")
	desc3 := testhelper.GetStringTvProto(t, "DescriptionThree")

	prio100 := int32(100)
	prio50 := int32(50)

	owner1 := "OwnerOne"
	owner2 := "OwnerTwo"

	ts1 := int64(9999999)

	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "9", "description"}, desc1, prio100, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc2, prio100, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner2, ts1)

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	// start test
	for _, u := range []*cache.Update{u1, u2, u3} {
		err := root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Test 1 - expect 2 entry for owner1", func(t *testing.T) {
		o1Le := []*LeafEntry{}
		o1Le = root.GetByOwner(owner1, o1Le)
		o1 := LeafEntriesToCacheUpdates(o1Le)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u2, u1}, o1); diff != "" {
			t.Errorf("root.GetByOwner(owner1) mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Test 2 - expect 1 entry for owner2", func(t *testing.T) {
		o2Le := []*LeafEntry{}
		o2Le = root.GetByOwner(owner2, o2Le)
		o2 := LeafEntriesToCacheUpdates(o2Le)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u3}, o2); diff != "" {
			t.Errorf("root.GetByOwner(owner2) mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Test 3 - GetHighesPrio()", func(t *testing.T) {

		highprec := root.GetHighestPrecedence(true)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u1, u3}, highprec); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})
}

// Test_Entry_Two adding a new Update with same owner and priority but updating the value
func Test_Entry_Two(t *testing.T) {

	desc3 := testhelper.GetStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)
	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner1, ts1)

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	// start test add "existing" data
	for _, u := range []*cache.Update{u1} {
		err := root.AddCacheUpdateRecursive(ctx, u, false)
		if err != nil {
			t.Error(err)
		}
	}

	// add incomming set intent reques data
	overwriteDesc := testhelper.GetStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1} {
		err := root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())
	highprec := root.GetHighestPrecedence(true)

	// diff the result with the expected
	if diff := testhelper.DiffCacheUpdates([]*cache.Update{n1}, highprec); diff != "" {
		t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
	}
}

// Test_Entry_Three Checks that an Intent update is processed properly
func Test_Entry_Three(t *testing.T) {
	desc3 := testhelper.GetStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)
	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, desc3, prio50, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "12", "description"}, desc3, prio50, owner1, ts1)
	u4 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "13", "description"}, desc3, prio50, owner1, ts1)

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	// start test add "existing" data
	for _, u := range []*cache.Update{u1, u2, u3, u4} {
		err := root.AddCacheUpdateRecursive(ctx, u, false)
		if err != nil {
			t.Error(err)
		}
	}

	t.Run("Check the data is present", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highpri := root.GetHighestPrecedence(false)

		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u1, u2, u3, u4}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check non is reported as New or Updated", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highpri := root.GetHighestPrecedence(true)

		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	// indicate that the intent is receiving an update
	// therefor invalidate all the present entries of the owner / intent
	root.MarkOwnerDelete(owner1)

	// add incomming set intent reques data
	overwriteDesc := testhelper.GetStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)
	n2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1, n2} {
		err := root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Check the original intent data of owner 1 is gone", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highPriLe := root.getByOwnerFiltered(owner1, FilterNonDeleted)

		highPri := LeafEntriesToCacheUpdates(highPriLe)

		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{n1, n2}, highPri); diff != "" {
			t.Errorf("root.GetByOwner() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone", func(t *testing.T) {
		highpri := root.GetHighestPrecedence(true)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{n1, n2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

}

// Test_Entry_Four Checks that an Intent update is processed properly with an intent that is shadowed initially.
func Test_Entry_Four(t *testing.T) {
	desc3 := testhelper.GetStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	prio55 := int32(55)
	owner1 := "OwnerOne"
	owner2 := "OwnerTwo"
	ts1 := int64(9999999)

	u1o1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner1, ts1)
	u2o1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, desc3, prio50, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "12", "description"}, desc3, prio50, owner1, ts1)
	u4 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "13", "description"}, desc3, prio50, owner1, ts1)

	u1o2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio55, owner2, ts1)
	u2o2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, desc3, prio55, owner2, ts1)

	ctx := context.Background()

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Error(err)
	}

	// start test add "existing" data
	for _, u := range []*cache.Update{u1o1, u2o1, u3, u4, u1o2, u2o2} {
		err := root.AddCacheUpdateRecursive(ctx, u, false)
		if err != nil {
			t.Error(err)
		}
	}

	t.Run("Check the data is present", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highprec := root.GetHighestPrecedence(false)

		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u1o1, u2o1, u3, u4}, highprec); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	// indicate that the intent is receiving an update
	// therefor invalidate all the present entries of the owner / intent
	root.MarkOwnerDelete(owner1)

	// add incomming set intent reques data
	overwriteDesc := testhelper.GetStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/1", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)
	n2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/1", "subinterface", "11", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1, n2} {
		err := root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Check the data is gone from ByOwner with NonDelete Filter", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highPriLe := root.getByOwnerFiltered(owner1, FilterNonDeleted)

		highPri := LeafEntriesToCacheUpdates(highPriLe)

		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{n1, n2}, highPri); diff != "" {
			t.Errorf("root.GetByOwner() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone from highest", func(t *testing.T) {
		highpri := root.GetHighestPrecedence(true)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{n1, n2, u1o2, u2o2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone from highest (only New Or Updated)", func(t *testing.T) {
		highpri := root.GetHighestPrecedence(true)
		// diff the result with the expected
		if diff := testhelper.DiffCacheUpdates([]*cache.Update{u1o2, u2o2, n1, n2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})
}

func Test_Entry_Delete_Aggregation(t *testing.T) {
	desc3 := testhelper.GetStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)

	u1 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, desc3, prio50, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/0"), prio50, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "0", "index"}, testhelper.GetStringTvProto(t, "0"), prio50, owner1, ts1)
	u4 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "0", "description"}, desc3, prio50, owner1, ts1)
	u5 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, testhelper.GetStringTvProto(t, "1"), prio50, owner1, ts1)
	u6 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, desc3, prio50, owner1, ts1)

	dev1 := &sdcio_schema.Device{
		Interface: map[string]*sdcio_schema.SdcioModel_Interface{},
	}

	dev1.Interface["ethernet-1/1"] = &sdcio_schema.SdcioModel_Interface{
		Name:        ygot.String("ethernet-1/1"),
		Description: ygot.String("DescriptionThree"),
	}

	json, err := ygot.EmitJSON(dev1, &ygot.EmitJSONConfig{
		Format: ygot.RFC7951,
		Indent: "  ",
		RFC7951Config: &ygot.RFC7951JSONConfig{
			AppendModuleName: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(json)

	ctx := context.Background()

	scb, err := getSchemaClientBound(t)
	if err != nil {
		t.Fatal(err)
	}

	tc := NewTreeContext(NewTreeSchemaCacheClient("dev1", nil, scb), "foo")

	root, err := NewTreeRoot(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}

	// start test add "existing" data
	for _, u := range []*cache.Update{u1, u2, u3, u4, u5, u6} {
		err := root.AddCacheUpdateRecursive(ctx, u, false)
		if err != nil {
			t.Fatal(err)
		}
	}

	// get ready to add the new intent data
	root.MarkOwnerDelete(owner1)

	u1n := cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, desc3, prio50, owner1, ts1)
	u2n := cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, testhelper.GetStringTvProto(t, "ethernet-0/1"), prio50, owner1, ts1)

	// start test add "new" / request data
	for _, u := range []*cache.Update{u1n, u2n} {
		err := root.AddCacheUpdateRecursive(ctx, u, true)
		if err != nil {
			t.Fatal(err)
		}
	}

	// retrieve the Deletes
	deletesSlices := root.GetDeletes()

	// process the result for comparison
	deletes := make([]string, 0, len(deletesSlices))
	for _, x := range deletesSlices {
		deletes = append(deletes, strings.Join(x, "/"))
	}

	// define the expected result
	expects := []string{
		"interface/ethernet-0/0",
	}
	// sort both slices for equality check
	slices.Sort(deletes)
	slices.Sort(expects)

	// perform comparison
	if diff := cmp.Diff(expects, deletes); diff != "" {
		t.Errorf("root.GetDeletes() mismatch (-want +got):\n%s", diff)
	}
}

// getSchemaClientBound creates a SchemaClientBound mock that responds to certain GetSchema requests
func getSchemaClientBound(t *testing.T) (SchemaClient.SchemaClientBound, error) {

	x, schema, err := testhelper.InitSDCIOSchema()
	if err != nil {
		return nil, err
	}

	mockCtrl := gomock.NewController(t)
	mockscb := mockschemaclientbound.NewMockSchemaClientBound(mockCtrl)

	// make the mock respond to GetSchema requests
	mockscb.EXPECT().GetSchema(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			return x.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:   path,
				Schema: schema,
			})
		},
	)

	// setup the ToPath() responses
	mockscb.EXPECT().ToPath(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, path []string) (*sdcpb.Path, error) {
			pr, err := x.ToPath(ctx, &sdcpb.ToPathRequest{
				PathElement: path,
				Schema:      schema,
			})
			if err != nil {
				return nil, err
			}
			return pr.GetPath(), nil
		},
	)

	// return the mock
	return mockscb, nil
}

// TestLeafVariants_GetHighesPrio
func TestLeafVariants_GetHighesPrio(t *testing.T) {
	owner1 := "owner1"
	ts := int64(0)
	path := []string{"firstPathElem"}

	// test that if highes prio is to be deleted, that the second highes is returned,
	// because thats an update.
	t.Run("Delete Non New",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 2, owner1, ts),
					IsUpdated: false,
					IsNew:     false,
					Delete:    false,
				},
				&LeafEntry{
					Update: cache.NewUpdate(path, nil, 1, owner1, ts),
					Delete: true,
				},
			}

			le := lv.GetHighestPrecedence(true)

			if le != lv[0] {
				t.Errorf("expected to get entry %v, got %v", lv[0], le)
			}

		},
	)

	// Have only a single entry in the list, thats marked for deletion
	// should return nil
	t.Run("Single entry thats also marked for deletion",
		func(t *testing.T) {

			lv := LeafVariants{
				&LeafEntry{
					Update: cache.NewUpdate(path, nil, 1, owner1, ts),
					Delete: true,
				},
			}

			le := lv.GetHighestPrecedence(true)

			if le != nil {
				t.Errorf("expected to get entry %v, got %v", nil, le)
			}
		},
	)

	// A preferred entry does exist the lower prio entry is updated.
	// on onlyIfPrioChanged == true we do not expect output.
	t.Run("New Low Prio IsUpdate OnlyChanged True",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 5, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: false,
				},
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 6, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: true,
				},
			}

			le := lv.GetHighestPrecedence(true)

			if le != nil {
				t.Errorf("expected to get entry %v, got %v", nil, le)
			}
		},
	)

	// A preferred entry does exist the lower prio entry is updated.
	// on onlyIfPrioChanged == false we do not expect the highes prio update to be returned.
	t.Run("New Low Prio IsUpdate OnlyChanged False",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 5, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: false,
				},
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 6, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: true,
				},
			}

			le := lv.GetHighestPrecedence(false)

			if le != lv[0] {
				t.Errorf("expected to get entry %v, got %v", lv[0], le)
			}
		},
	)

	// A preferred entry does exist the lower prio entry is new.
	// on onlyIfPrioChanged == true we do not expect output.
	t.Run("New Low Prio IsNew OnlyChanged == True",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 5, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: false,
				},
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 6, owner1, ts),
					Delete:    false,
					IsNew:     true,
					IsUpdated: false,
				},
			}

			le := lv.GetHighestPrecedence(true)

			if le != nil {
				t.Errorf("expected to get entry %v, got %v", nil, le)
			}
		},
	)

	t.Run("New Low Prio IsNew OnlyChanged == False",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 5, owner1, ts),
					Delete:    false,
					IsNew:     false,
					IsUpdated: false,
				},
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 6, owner1, ts),
					Delete:    false,
					IsNew:     true,
					IsUpdated: false,
				},
			}

			le := lv.GetHighestPrecedence(false)

			if le != lv[0] {
				t.Errorf("expected to get entry %v, got %v", lv[0], le)
			}
		},
	)

	// If no entries exist in the list nil should be returned.
	t.Run("No Entries",
		func(t *testing.T) {
			lv := LeafVariants{}

			le := lv.GetHighestPrecedence(true)

			if le != nil {
				t.Errorf("expected to get entry %v, got %v", nil, le)
			}
		},
	)

	// make sure the secondhighes is also populated if the highes was the first entry
	t.Run("No Entries",
		func(t *testing.T) {
			lv := LeafVariants{
				&LeafEntry{
					Update: cache.NewUpdate(path, nil, 1, owner1, ts),
					Delete: true,
				},
				&LeafEntry{
					Update:    cache.NewUpdate(path, nil, 2, owner1, ts),
					IsUpdated: false,
					IsNew:     false,
					Delete:    false,
				},
			}

			le := lv.GetHighestPrecedence(true)

			if le != lv[1] {
				t.Errorf("expected to get entry %v, got %v", lv[1], le)
			}
		},
	)
}
