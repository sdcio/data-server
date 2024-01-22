package ctree

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iptecharch/schema-server/pkg/config"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/openconfig/gnmi/path"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"

	"github.com/iptecharch/data-server/pkg/utils"
)

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

func (t *Tree) AddGNMINotification(n *gnmi.Notification) error {
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
		// convert gnmi.TypedValue to sdcpb.TypedValue
		scVal := utils.ToSchemaTypedValue(upd.GetVal())
		err = t.Add(items, scVal)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tree) AddSchemaNotification(n *sdcpb.Notification) error {
	if n == nil {
		return nil
	}

	for _, del := range n.GetDelete() {
		items, err := utils.CompletePath(nil, del)
		if err != nil {
			return err
		}
		r := t.Delete(items)
		log.Debugf("deleted %v", r)
	}

	for _, upd := range n.GetUpdate() {
		err := t.AddSchemaUpdate(upd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tree) AddNotification(n any) error {
	switch n := n.(type) {
	case *gnmi.Notification:
		return t.AddGNMINotification(n)
	case *sdcpb.Notification:
		return t.AddSchemaNotification(n)
	default:
		return fmt.Errorf("unknown notification type %T", n)
	}
}

func (t *Tree) AddSchemaUpdate(upd *sdcpb.Update) error {
	items, err := utils.CompletePath(nil, upd.GetPath())
	if err != nil {
		return err
	}
	log.Debugf("adding value: %T, %v into path %v", upd.GetValue(), upd.GetValue(), items)
	err = t.Add(items, upd.GetValue())
	return err
}

func (t *Tree) GetPath(ctx context.Context, p *sdcpb.Path, schemaClient sdcpb.SchemaServerClient, sc *config.SchemaConfig) ([]*sdcpb.Notification, error) {
	cp, err := utils.CompletePath(nil, p)
	if err != nil {
		return nil, err
	}
	return t.GetNotifications(ctx, cp, schemaClient, sc)
}

func (t *Tree) GetNotifications(ctx context.Context, p []string, schemaClient sdcpb.SchemaServerClient, sc *config.SchemaConfig) ([]*sdcpb.Notification, error) {
	ns := make([]*sdcpb.Notification, 0)
	err := t.Query(p,
		func(path []string, _ *Leaf, val interface{}) error {
			req := &sdcpb.ToPathRequest{
				PathElement: path,
				Schema:      sc.GetSchema(),
			}
			rsp, err := schemaClient.ToPath(ctx, req)
			if err != nil {
				return err
			}
			n := &sdcpb.Notification{
				Timestamp: time.Now().UnixNano(),
				Update: []*sdcpb.Update{{
					Path:  rsp.GetPath(),
					Value: utils.ToSchemaTypedValue(val),
				}},
			}
			ns = append(ns, n)
			return nil
		})
	return ns, err
}

func (t *Tree) DeletePath(p *sdcpb.Path) error {
	cp, err := utils.CompletePath(nil, p)
	if err != nil {
		return err
	}
	t.Delete(cp)
	return nil
}

func (t *Tree) PrintTree() string {
	sb := strings.Builder{}
	t.WalkSorted(
		func(path []string, _ *Leaf, val interface{}) error {
			sb.WriteString(strings.Join(path, "/"))
			sb.WriteString(": ")
			sb.WriteString(fmt.Sprintf("%T: %s", val, val))
			sb.WriteString("\n")
			return nil
		})
	return sb.String()
}

func (t *Tree) Count() uint64 {
	var c uint64
	t.Walk(
		func(_ []string, _ *Leaf, _ interface{}) error {
			c++
			return nil
		})
	return c
}
