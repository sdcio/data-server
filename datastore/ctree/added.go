package ctree

import (
	"encoding/json"
	"fmt"

	"github.com/openconfig/gnmi/path"
	"github.com/openconfig/gnmi/proto/gnmi"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

func (t *Tree) PrettyJSON() ([]byte, error) {
	if t == nil || t.leafBranch == nil {
		return nil, nil
	}
	var o interface{}
	err := json.Unmarshal([]byte(t.String()), &o)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(o, "", "  ")

}

func (t *Tree) Clone() (*Tree, error) {
	nt := &Tree{}
	err := t.Walk(func(path []string, _ *Leaf, val interface{}) error {
		return nt.Add(path, val)
	})
	return nt, err
}

func (t *Tree) Merge(nt *Tree) error {
	return nt.Walk(func(path []string, _ *Leaf, val interface{}) error {
		return t.Add(path, val)
	})
}

func (t *Tree) AddGNMIUpdate(n *gnmi.Notification) error {
	if n == nil {
		return nil
	}

	for _, del := range n.GetDelete() {
		items, err := path.CompletePath(n.GetPrefix(), del)
		if err != nil {
			return err
		}
		r := t.Delete(items)
		fmt.Printf("deleted :%#v\n", r)
	}
	for _, upd := range n.GetUpdate() {
		items, err := path.CompletePath(n.GetPrefix(), upd.GetPath())
		if err != nil {
			return err
		}
		err = t.Add(items, upd.GetVal().Value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tree) AddSchemaUpdate(n *schemapb.Notification) error {
	if n == nil {
		return nil
	}

	for _, del := range n.GetDelete() {
		items, err := CompletePath(n.GetPrefix(), del)
		if err != nil {
			return err
		}
		r := t.Delete(items)
		fmt.Printf("deleted :%#v\n", r)
	}

	for _, upd := range n.GetUpdate() {
		items, err := CompletePath(n.GetPrefix(), upd.GetPath())
		if err != nil {
			return err
		}
		err = t.Add(items, upd.GetValue().Value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tree) AddUpdate(n any) error {
	switch n := n.(type) {
	case *gnmi.Notification:
		return t.AddGNMIUpdate(n)
	case *schemapb.Notification:
		return t.AddSchemaUpdate(n)
	default:
		return fmt.Errorf("unknown update type %T", n)
	}
}

func (t *Tree) GetPath(p *schemapb.Path, schemaClient schemapb.SchemaServerClient, sc *config.SchemaConfig) ([]*schemapb.Notification, error) {
	return nil, nil
}

func (t *Tree) DeletePath(p *schemapb.Path) error { return nil } // TODO2:
func (t *Tree) Insert(upd *schemapb.Update) error { return nil } // TODO2:
