package ctree

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	"github.com/openconfig/gnmi/path"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
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
		log.Debugf("deleted :%#v", r)
	}
	for _, upd := range n.GetUpdate() {
		items, err := path.CompletePath(n.GetPrefix(), upd.GetPath())
		if err != nil {
			return err
		}
		if upd.GetVal().GetValue() == nil {
			continue
		}
		err = t.Add(items, upd.GetVal().GetValue())
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
		items, err := utils.CompletePath(n.GetPrefix(), del)
		if err != nil {
			return err
		}
		r := t.Delete(items)
		log.Debugf("deleted %v", r)
	}

	for _, upd := range n.GetUpdate() {
		items, err := utils.CompletePath(n.GetPrefix(), upd.GetPath())
		if err != nil {
			return err
		}
		v := utils.ToGNMITypedValue(upd.GetValue().GetValue())
		fmt.Printf("adding value: %T, %v\n", upd.GetValue().GetValue(), upd.GetValue().GetValue())
		err = t.Add(items, v)
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

func (t *Tree) GetPath(ctx context.Context, p *schemapb.Path, schemaClient schemapb.SchemaServerClient, sc *config.SchemaConfig) ([]*schemapb.Notification, error) {
	cp, err := utils.CompletePath(nil, p)
	if err != nil {
		return nil, err
	}
	ns := make([]*schemapb.Notification, 0)
	err = t.Query(cp,
		func(path []string, l *Leaf, val interface{}) error {
			req := &schemapb.ToPathRequest{
				PathElement: path,
				Schema: &schemapb.Schema{
					Name:    sc.Name,
					Vendor:  sc.Vendor,
					Version: sc.Version,
				},
			}
			fmt.Println("GetPath, Query, ToPath", req)
			rsp, err := schemaClient.ToPath(ctx, req)
			if err != nil {
				return err
			}
			fmt.Println(rsp)
			n := &schemapb.Notification{
				Timestamp: time.Now().UnixNano(),
				Update: []*schemapb.Update{{
					Path:  rsp.GetPath(),
					Value: utils.ToSchemaTypedValue(val),
				}},
			}
			ns = append(ns, n)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (t *Tree) DeletePath(p *schemapb.Path) error {
	cp, err := utils.CompletePath(nil, p)
	if err != nil {
		return err
	}
	t.Delete(cp)
	return nil
}

func (t *Tree) Insert(upd *schemapb.Update) error {
	cp, err := utils.CompletePath(nil, upd.GetPath())
	if err != nil {
		return err
	}
	return t.Add(cp, upd.GetValue().GetValue())
}
