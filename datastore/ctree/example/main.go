package main

import (
	"fmt"

	"github.com/iptecharch/schema-server/datastore/ctree"
)

var data1 = []struct {
	path  []string
	value any
}{
	{
		path:  []string{"a", "b1", "c1"},
		value: 1,
	},
	{
		path:  []string{"a", "b1", "c2"},
		value: 2,
	},
	{
		path:  []string{"a", "b1", "c3", "d"},
		value: 3,
	},
	{
		path:  []string{"a", "b2", "c1"},
		value: 1,
	},
	{
		path:  []string{"a", "b2", "c2"},
		value: 2,
	},
	{
		path:  []string{"a", "b2", "c3"},
		value: 3,
	},
}

var data2 = []struct {
	path  []string
	value any
}{
	{
		path:  []string{"a", "b1", "c1"},
		value: 1,
	},
	{
		path:  []string{"a", "b1", "c2"},
		value: 2,
	},
	{
		path:  []string{"a", "b1", "c3", "d"},
		value: 3,
	},
	{
		path:  []string{"a", "b2", "c1"},
		value: 1,
	},
	{
		path:  []string{"a", "b2", "c2"},
		value: 2,
	},
	{
		path:  []string{"a", "b2", "c3"},
		value: 3,
	},
	{
		path:  []string{"a", "b2", "c4", "d"},
		value: 42,
	},
}

func main() {
	var err error
	tr1 := &ctree.Tree{}
	for _, e := range data1 {
		err = tr1.Add(e.path, e.value)
		if err != nil {
			panic(err)
		}
	}
	b1, err := tr1.PrettyJSON()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b1))
	tr2 := &ctree.Tree{}
	for _, e := range data2 {
		err = tr2.Add(e.path, e.value)
		if err != nil {
			panic(err)
		}
	}
	b2, err := tr2.PrettyJSON()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b2))

	nt, err := tr1.Clone()
	if err != nil {
		panic(err)
	}
	bb, err := nt.PrettyJSON()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(bb))
	err = tr1.Merge(tr2)
	if err != nil {
		panic(err)
	}
	bbb, err := tr1.PrettyJSON()
	if err != nil {
		panic(err)
	}
	fmt.Println("merge:")
	fmt.Println(string(bbb))
}
