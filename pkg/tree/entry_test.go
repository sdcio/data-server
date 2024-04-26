package tree

import (
	"strings"
	"testing"

	"github.com/sdcio/data-server/pkg/cache"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
		highpri := root.GetHighesPrio()
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

	highpri := root.GetHighesPrio()

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

		highpri := root.GetHighesPrio()

		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{u1, u2, u3, u4}, highpri); diff != "" {
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
		highpri := root.GetHighesPrio()
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

		highpri := root.GetHighesPrio()

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
		highpri := root.GetHighesPrio()
		// diff the result with the expected
		if diff := diffCacheUpdates([]*cache.Update{n1, n2, u1o2, u2o2}, highpri); diff != "" {
			t.Errorf("root.GetHighesPrio() mismatch (-want +got):\n%s", diff)
		}
	})

}
