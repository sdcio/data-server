package tree

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sdcio/data-server/mocks/mockschemaclientbound"
	"github.com/sdcio/data-server/pkg/cache"
	SchemaClient "github.com/sdcio/data-server/pkg/datastore/clients/schema"
	"github.com/sdcio/data-server/pkg/utils"
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

	root := NewTreeRoot()

	for _, u := range []*cache.Update{u1, u2, u3} {
		err = root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	r := []string{}
	r = root.StringIndent(r)
	t.Log(strings.Join(r, "\n"))
}

func Test_Entry_One(t *testing.T) {
	desc1 := getStringTvProto(t, "DescriptionOne")
	desc2 := getStringTvProto(t, "DescriptionTwo")
	desc3 := getStringTvProto(t, "DescriptionThree")

	prio100 := int32(100)
	prio50 := int32(50)

	owner1 := "OwnerOne"
	owner2 := "OwnerTwo"

	ts1 := int64(9999999)

	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "9", "description"}, desc1, prio100, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc2, prio100, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner2, ts1)

	root := NewTreeRoot()

	// start test
	for _, u := range []*cache.Update{u1, u2, u3} {
		err := root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Test 1 - expect 2 entry for owner1", func(t *testing.T) {
		o1Le := root.GetByOwner(owner1)
		o1 := LeafEntriesToCacheUpdates(o1Le)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u2, u1}, o1); diff != "" {
			t.Errorf("root.GetByOwner(owner1) mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Test 2 - expect 1 entry for owner2", func(t *testing.T) {
		o2Le := root.GetByOwner(owner2)
		o2 := LeafEntriesToCacheUpdates(o2Le)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u3}, o2); diff != "" {
			t.Errorf("root.GetByOwner(owner2) mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Test 3 - GetHighesPrio()", func(t *testing.T) {
		highpri := root.GetHighesPrio(true)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u1, u3}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})
}

// Test_Entry_Two adding a new Update with same owner and priority but updating the value
func Test_Entry_Two(t *testing.T) {

	desc3 := getStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)
	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner1, ts1)

	root := NewTreeRoot()

	// start test add "existing" data
	for _, u := range []*cache.Update{u1} {
		err := root.AddCacheUpdateRecursive(u, false)
		if err != nil {
			t.Error(err)
		}
	}

	// add incomming set intent reques data
	overwriteDesc := getStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1} {
		err := root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	highpri := root.GetHighesPrio(true)

	// diff the result with the expected
	if diff := diffCacheUpdates([]*cache.Update{n1}, highpri); diff != "" {
		t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
	}
}

// Test_Entry_Three Checks that an Intent update is processed properly
func Test_Entry_Three(t *testing.T) {
	desc3 := getStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)
	u1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, desc3, prio50, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, desc3, prio50, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "12", "description"}, desc3, prio50, owner1, ts1)
	u4 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "13", "description"}, desc3, prio50, owner1, ts1)

	root := NewTreeRoot()

	// start test add "existing" data
	for _, u := range []*cache.Update{u1, u2, u3, u4} {
		err := root.AddCacheUpdateRecursive(u, false)
		if err != nil {
			t.Error(err)
		}
	}

	t.Run("Check the data is present", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highpri := root.GetHighesPrio(false)

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u1, u2, u3, u4}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check non is reported as New or Updated", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highpri := root.GetHighesPrio(true)

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	// indicate that the intent is receiving an update
	// therefor invalidate all the present entries of the owner / intent
	root.MarkOwnerDelete(owner1)

	// add incomming set intent reques data
	overwriteDesc := getStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)
	n2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/0", "subinterface", "11", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1, n2} {
		err := root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Check the original intent data of owner 1 is gone", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highPriLe := root.GetByOwnerFiltered(owner1, FilterNonDeleted)

		highPri := LeafEntriesToCacheUpdates(highPriLe)

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2}, highPri); diff != "" {
			t.Errorf("root.GetByOwner() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone", func(t *testing.T) {
		highpri := root.GetHighesPrio(true)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

}

// Test_Entry_Four Checks that an Intent update is processed properly with an intent that is shadowed initially.
func Test_Entry_Four(t *testing.T) {
	desc3 := getStringTvProto(t, "DescriptionThree")
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

	root := NewTreeRoot()

	// start test add "existing" data
	for _, u := range []*cache.Update{u1o1, u2o1, u3, u4, u1o2, u2o2} {
		err := root.AddCacheUpdateRecursive(u, false)
		if err != nil {
			t.Error(err)
		}
	}

	t.Run("Check the data is present", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highpri := root.GetHighesPrio(false)

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u1o1, u2o1, u3, u4}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	// indicate that the intent is receiving an update
	// therefor invalidate all the present entries of the owner / intent
	root.MarkOwnerDelete(owner1)

	// add incomming set intent reques data
	overwriteDesc := getStringTvProto(t, "Owerwrite Description")

	// adding a new Update with same owner and priority with different value
	n1 := cache.NewUpdate([]string{"interfaces", "ethernet-0/1", "subinterface", "10", "description"}, overwriteDesc, prio50, owner1, ts1)
	n2 := cache.NewUpdate([]string{"interfaces", "ethernet-0/1", "subinterface", "11", "description"}, overwriteDesc, prio50, owner1, ts1)

	for _, u := range []*cache.Update{n1, n2} {
		err := root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// log the tree
	t.Log(root.String())

	t.Run("Check the data is gone from ByOwner with NonDelete Filter", func(t *testing.T) {

		// log the tree
		t.Log(root.String())

		highPriLe := root.GetByOwnerFiltered(owner1, FilterNonDeleted)

		highPri := LeafEntriesToCacheUpdates(highPriLe)

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2}, highPri); diff != "" {
			t.Errorf("root.GetByOwner() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone from highest", func(t *testing.T) {
		highpri := root.GetHighesPrio(false)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2, u1o2, u2o2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("Check the old entries are gone from highest (only New Or Updated)", func(t *testing.T) {
		highpri := root.GetHighesPrio(true)
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})
}

func Test_Entry_Delete_Aggregation(t *testing.T) {
	desc3 := getStringTvProto(t, "DescriptionThree")
	prio50 := int32(50)
	owner1 := "OwnerOne"
	ts1 := int64(9999999)

	u1 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "description"}, desc3, prio50, owner1, ts1)
	u2 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "name"}, getStringTvProto(t, "ethernet-0/0"), prio50, owner1, ts1)
	u3 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "0", "index"}, getStringTvProto(t, "0"), prio50, owner1, ts1)
	u4 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "0", "description"}, desc3, prio50, owner1, ts1)
	u5 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "index"}, getStringTvProto(t, "1"), prio50, owner1, ts1)
	u6 := cache.NewUpdate([]string{"interface", "ethernet-0/0", "subinterface", "1", "description"}, desc3, prio50, owner1, ts1)

	root := NewTreeRoot()

	// start test add "existing" data
	for _, u := range []*cache.Update{u1, u2, u3, u4, u5, u6} {
		err := root.AddCacheUpdateRecursive(u, false)
		if err != nil {
			t.Error(err)
		}
	}

	// get ready to add the new intent data
	root.MarkOwnerDelete(owner1)

	u1n := cache.NewUpdate([]string{"interface", "ethernet-0/1", "description"}, desc3, prio50, owner1, ts1)
	u2n := cache.NewUpdate([]string{"interface", "ethernet-0/1", "name"}, getStringTvProto(t, "ethernet-0/1"), prio50, owner1, ts1)

	// start test add "new" / request data
	for _, u := range []*cache.Update{u1n, u2n} {
		err := root.AddCacheUpdateRecursive(u, true)
		if err != nil {
			t.Error(err)
		}
	}

	// get a visitor with the schemaClientBound mock
	visitor := TreeWalkerSchemaRetriever(context.Background(), getSchemaClientBound(t))

	// populate the tree with the schema entries
	root.Walk(visitor)

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
func getSchemaClientBound(t *testing.T) SchemaClient.SchemaClientBound {
	mockCtrl := gomock.NewController(t)

	mockscb := mockschemaclientbound.NewMockSchemaClientBound(mockCtrl)

	// index for the ToPath() function
	responseMap := map[string]*sdcpb.SchemaElem{}

	// root
	responseMap[""] = createSchemaContainer("__root__", nil)

	// interface
	responseMap["/interface"] = createSchemaContainer("interface", []string{"name"})
	responseMap["/interface/name"] = createSchemaField("name")
	responseMap["/interface/description"] = createSchemaField("description")

	// interface / subinterface
	responseMap["/interface/subinterface"] = createSchemaContainer("subinterface", []string{"index"})
	responseMap["/interface/subinterface/index"] = createSchemaField("index")
	responseMap["/interface/subinterface/description"] = createSchemaField("description")

	// network-instance
	responseMap["/network-instance"] = createSchemaContainer("network-instance", []string{"name"})
	responseMap["/network-instance/name"] = createSchemaField("name")
	responseMap["/network-instance/description"] = createSchemaField("description")

	// make the mock respond to GetSchema requests
	mockscb.EXPECT().GetSchema(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, path *sdcpb.Path) (*sdcpb.GetSchemaResponse, error) {
			keylessString := utils.SdcpbPathToKeylessString(path)
			return &sdcpb.GetSchemaResponse{
				Schema: responseMap[keylessString],
			}, nil
		},
	)

	// index for the ToPath() function
	pathMap := map[string]*sdcpb.Path{}
	pathMap[""] = &sdcpb.Path{}

	pathMap["interface"] = createPath(pathMap[""], "interface", nil)

	// interface ethernet0/0
	pathMap["interface/ethernet-0/0"] = createPath(pathMap["interface"], "interface", map[string]string{"name": "ethernet-0/0"})

	pathMap["interface/ethernet-0/0/name"] = createPath(pathMap["interface/ethernet-0/0"], "name", nil)
	pathMap["interface/ethernet-0/0/description"] = createPath(pathMap["interface/ethernet-0/0"], "description", nil)

	pathMap["interface/ethernet-0/0/subinterface"] = createPath(pathMap["interface/ethernet-0/0"], "subinterface", nil)
	pathMap["interface/ethernet-0/0/subinterface/0"] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "0"})
	pathMap["interface/ethernet-0/0/subinterface/0/index"] = createPath(pathMap["interface/ethernet-0/0/subinterface/0"], "index", nil)
	pathMap["interface/ethernet-0/0/subinterface/0/description"] = createPath(pathMap["interface/ethernet-0/0/subinterface/0"], "description", nil)
	pathMap["interface/ethernet-0/0/subinterface/1"] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "1"})
	pathMap["interface/ethernet-0/0/subinterface/1/index"] = createPath(pathMap["interface/ethernet-0/0/subinterface/1"], "index", nil)
	pathMap["interface/ethernet-0/0/subinterface/1/description"] = createPath(pathMap["interface/ethernet-0/0/subinterface/1"], "description", nil)
	pathMap["interface/ethernet-0/0/subinterface/2"] = createPath(pathMap["interface/ethernet-0/0/subinterface"], "subinterface", map[string]string{"index": "2"})
	pathMap["interface/ethernet-0/0/subinterface/2/index"] = createPath(pathMap["interface/ethernet-0/0/subinterface/2"], "index", nil)
	pathMap["interface/ethernet-0/0/subinterface/2/description"] = createPath(pathMap["interface/ethernet-0/0/subinterface/2"], "description", nil)

	// interface ethernet0/1
	pathMap["interface/ethernet-0/1"] = createPath(pathMap["interface"], "interface", map[string]string{"name": "ethernet-0/1"})

	pathMap["interface/ethernet-0/1/name"] = createPath(pathMap["interface/ethernet-0/1"], "name", nil)
	pathMap["interface/ethernet-0/1/description"] = createPath(pathMap["interface/ethernet-0/1"], "description", nil)

	pathMap["interface/ethernet-0/1/subinterface"] = createPath(pathMap["interface/ethernet-0/1"], "subinterface", nil)
	pathMap["interface/ethernet-0/1/subinterface/0"] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "0"})
	pathMap["interface/ethernet-0/1/subinterface/0/description"] = createPath(pathMap["interface/ethernet-0/1/subinterface/0"], "description", nil)
	pathMap["interface/ethernet-0/1/subinterface/0/index"] = createPath(pathMap["interface/ethernet-0/1/subinterface/0"], "index", nil)
	pathMap["interface/ethernet-0/1/subinterface/1"] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "1"})
	pathMap["interface/ethernet-0/1/subinterface/1/description"] = createPath(pathMap["interface/ethernet-0/1/subinterface/1"], "description", nil)
	pathMap["interface/ethernet-0/1/subinterface/1/index"] = createPath(pathMap["interface/ethernet-0/1/subinterface/1"], "index", nil)
	pathMap["interface/ethernet-0/1/subinterface/2"] = createPath(pathMap["interface/ethernet-0/1/subinterface"], "subinterface", map[string]string{"index": "2"})
	pathMap["interface/ethernet-0/1/subinterface/2/description"] = createPath(pathMap["interface/ethernet-0/1/subinterface/2"], "description", nil)
	pathMap["interface/ethernet-0/1/subinterface/2/index"] = createPath(pathMap["interface/ethernet-0/1/subinterface/2"], "index", nil)

	// setup the ToPath() responses
	mockscb.EXPECT().ToPath(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, path []string) (*sdcpb.Path, error) {
			return pathMap[strings.Join(path, "/")], nil
		},
	)

	// return the mock
	return mockscb
}

func createPath(p *sdcpb.Path, elemName string, keys map[string]string) *sdcpb.Path {
	result := utils.CopyPath(p)

	result.Elem = append(result.Elem, &sdcpb.PathElem{
		Name: elemName,
		Key:  keys,
	})
	return result
}

// createSchemaContainer generate a container schema elem
func createSchemaContainer(name string, keys []string) *sdcpb.SchemaElem {

	// process the keys
	sKeys := []*sdcpb.LeafSchema{}
	for _, k := range keys {
		sKeys = append(sKeys, &sdcpb.LeafSchema{
			Name: k,
		})
	}

	// build and return the schema element
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{
			Container: &sdcpb.ContainerSchema{
				Name: name,
				Keys: sKeys,
			},
		},
	}
}

// createSchemaField generate a field schema elem
func createSchemaField(name string) *sdcpb.SchemaElem {
	return &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Field{
			Field: &sdcpb.LeafSchema{
				Name: name,
			},
		},
	}
}
