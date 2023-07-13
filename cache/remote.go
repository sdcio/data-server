package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/iptecharch/cache/client"
	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/schema-server/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type remoteCache struct {
	c *client.Client
}

func NewRemoteCache(ctx context.Context, addr string) (Client, error) {
	cc, err := client.New(ctx, &client.ClientConfig{Address: addr})
	if err != nil {
		return nil, err
	}
	return &remoteCache{
		c: cc,
	}, nil
}

func (c *remoteCache) Create(ctx context.Context, name string, ephemeral bool, cached bool) error {
	return c.c.Create(ctx, name, ephemeral, cached)
}

func (c *remoteCache) List(ctx context.Context) ([]string, error) {
	return c.c.List(ctx)
}

func (c *remoteCache) GetCandidates(ctx context.Context, name string) ([]string, error) {
	rsp, err := c.c.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return rsp.GetCandidate(), nil
}

func (c *remoteCache) HasCandidate(ctx context.Context, name, candidate string) (bool, error) {
	cands, err := c.GetCandidates(ctx, name)
	if err != nil {
		return false, err
	}
	for _, cand := range cands {
		if cand == candidate {
			return true, nil
		}
	}
	return false, nil
}

func (c *remoteCache) Delete(ctx context.Context, name string) error {
	return c.c.Delete(ctx, name)
}

func (c *remoteCache) Exists(ctx context.Context, name string) (bool, error) {
	return c.c.Exists(ctx, name)
}

func (c *remoteCache) CreateCandidate(ctx context.Context, name, candidate string) error {
	return c.c.CreateCandidate(ctx, name, candidate)
}

func (c *remoteCache) DeleteCandidate(ctx context.Context, name, candidate string) error {
	return c.c.Delete(ctx, fmt.Sprintf("%s/%s", name, candidate))
}

func (c *remoteCache) Clone(ctx context.Context, name, clone string) error {
	return c.c.Clone(ctx, name, clone)
}

func (c *remoteCache) Modify(ctx context.Context, name string, store cachepb.Store, dels [][]string, upds []Update) error {
	pbUpds := make([]*cachepb.Update, 0, len(upds))
	for _, upd := range upds {
		pbUpds = append(pbUpds,
			&cachepb.Update{
				Path: upd.GetPath(),
				Value: &anypb.Any{
					Value: upd.Bytes(),
				},
			},
		)
	}
	return c.c.Modify(ctx, name, store, dels, pbUpds)
}

func (c *remoteCache) Read(ctx context.Context, name string, store cachepb.Store, paths [][]string, period time.Duration) []Update {
	outCh := c.ReadCh(ctx, name, store, paths, period)
	updates := make([]Update, 0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case upd, ok := <-outCh:
			if !ok {
				return updates
			}
			updates = append(updates, upd)
		}
	}
}

func (c *remoteCache) ReadCh(ctx context.Context, name string, store cachepb.Store, paths [][]string, period time.Duration) chan Update {
	inCh := c.c.Read(ctx, name, store, paths, period)
	outCh := make(chan Update)
	go func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				return
			case upd, ok := <-inCh:
				if !ok {
					return
				}
				rUpd := &remoteUpdate{
					path:  upd.GetPath(),
					bytes: upd.GetValue().GetValue(),
				}
				select {
				case <-ctx.Done():
					return
				case outCh <- rUpd:
				}
			}
		}
	}()
	return outCh
}

func (c *remoteCache) GetChanges(ctx context.Context, name, candidate string) ([]*Change, error) {
	changes, err := c.c.GetChanges(ctx, name, candidate)
	if err != nil {
		return nil, err
	}
	lcs := make([]*Change, 0, len(changes))
	for _, change := range changes {
		lcs = append(lcs, &Change{
			Update: &remoteUpdate{
				path:  change.GetUpdate().GetPath(),
				bytes: change.GetUpdate().GetValue().GetValue(),
			},
			Delete: change.GetDelete(),
		})
	}
	return lcs, nil
}

func (c *remoteCache) Discard(ctx context.Context, name, candidate string) error {
	return c.c.Discard(ctx, name, candidate)
}

func (c *remoteCache) NewUpdate(upd *sdcpb.Update) (Update, error) {
	b, err := proto.Marshal(upd.GetValue())
	if err != nil {
		return nil, err
	}
	rupd := &remoteUpdate{
		path:  utils.ToStrings(upd.GetPath(), false, false),
		bytes: b,
	}
	return rupd, nil
}

func (c *remoteCache) Close() error {
	return c.c.Close()
}

type remoteUpdate struct {
	path  []string
	bytes []byte
}

func (u *remoteUpdate) GetPath() []string {
	return u.path
}

func (u *remoteUpdate) Value() (*sdcpb.TypedValue, error) {
	tv := new(sdcpb.TypedValue)
	err := proto.Unmarshal(u.bytes, tv)
	if err != nil {
		return nil, err
	}
	return tv, nil
}

func (u *remoteUpdate) Bytes() []byte {
	return u.bytes
}
