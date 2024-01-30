package cache

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/iptecharch/cache/pkg/cache"
	"github.com/iptecharch/cache/pkg/client"
	"github.com/iptecharch/cache/proto/cachepb"
	"github.com/iptecharch/schema-server/pkg/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
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
	return c.c.Create(ctx, name)
}

func (c *remoteCache) List(ctx context.Context) ([]string, error) {
	return c.c.List(ctx)
}

func (c *remoteCache) GetCandidates(ctx context.Context, name string) ([]*cache.CandidateDetails, error) {
	rsp, err := c.c.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	rs := make([]*cache.CandidateDetails, 0, len(rsp.GetCandidate()))
	for _, candpb := range rsp.GetCandidate() {
		rs = append(rs, &cache.CandidateDetails{
			CacheName:     name,
			CandidateName: candpb.GetName(),
			Owner:         candpb.GetOwner(),
			Priority:      candpb.GetPriority(),
		})
	}
	return rs, nil
}

func (c *remoteCache) HasCandidate(ctx context.Context, name, candidate string) (bool, error) {
	cands, err := c.GetCandidates(ctx, name)
	if err != nil {
		return false, err
	}
	for _, cand := range cands {
		if cand.CandidateName == candidate {
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

func (c *remoteCache) CreateCandidate(ctx context.Context, name, candidate, owner string, priority int32) error {
	return c.c.CreateCandidate(ctx, name, candidate, owner, priority)
}

func (c *remoteCache) DeleteCandidate(ctx context.Context, name, candidate string) error {
	return c.c.Delete(ctx, fmt.Sprintf("%s/%s", name, candidate))
}

func (c *remoteCache) Clone(ctx context.Context, name, clone string) error {
	return c.c.Clone(ctx, name, clone)
}

func (c *remoteCache) Modify(ctx context.Context, name string, opts *Opts, dels [][]string, upds []*Update) error {
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

	wo := &client.ClientOpts{
		Owner:         opts.Owner,
		Priority:      opts.Priority,
		Store:         getStore(opts.Store),
		PriorityCount: opts.PriorityCount,
	}

	return c.c.Modify(ctx, name, wo, dels, pbUpds)
}

func (c *remoteCache) Read(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) []*Update {
	outCh := c.ReadCh(ctx, name, opts, paths, period)
	updates := make([]*Update, 0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case upd, ok := <-outCh:
			if !ok {
				sort.Slice(updates, func(i, j int) bool {
					return updates[i].ts < updates[j].ts
				})
				return updates
			}
			updates = append(updates, upd)
		}
	}
}

func (c *remoteCache) ReadCh(ctx context.Context, name string, opts *Opts, paths [][]string, period time.Duration) chan *Update {
	ro := &client.ClientOpts{
		Owner:         opts.Owner,
		Priority:      opts.Priority,
		Store:         getStore(opts.Store),
		PriorityCount: opts.PriorityCount,
		KeysOnly:      opts.KeysOnly,
	}
	inCh := c.c.Read(ctx, name, ro, paths, period)
	outCh := make(chan *Update, len(paths))
	go func() {
		defer close(outCh)
		for {
			select {
			case <-ctx.Done():
				if !errors.Is(ctx.Err(), context.Canceled) {
					log.Errorf("ctx done: %v", ctx.Err())
				}
				return
			case readResponse, ok := <-inCh:
				if !ok {
					return
				}
				rUpd := &Update{
					path:     readResponse.GetPath(),
					value:    readResponse.GetValue().GetValue(),
					priority: readResponse.GetPriority(),
					owner:    readResponse.GetOwner(),
					ts:       readResponse.GetTimestamp(),
				}
				select {
				case <-ctx.Done():
					if !errors.Is(ctx.Err(), context.Canceled) {
						log.Errorf("ctx done: %v", ctx.Err())
					}
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
			Update: &Update{
				path:  change.GetUpdate().GetPath(),
				value: change.GetUpdate().GetValue().GetValue(),
			},
			Delete: change.GetDelete(),
		})
	}
	return lcs, nil
}

func (c *remoteCache) Discard(ctx context.Context, name, candidate string) error {
	return c.c.Discard(ctx, name, candidate)
}

func (c *remoteCache) Commit(ctx context.Context, name, candidate string) error {
	return c.c.Commit(ctx, name, candidate)
}

func (c *remoteCache) CreatePruneID(ctx context.Context, name string, force bool) (string, error) {
	rsp, err := c.c.Prune(ctx, name, "", force)
	if err != nil {
		return "", err
	}
	return rsp.GetId(), nil
}

func (c *remoteCache) ApplyPrune(ctx context.Context, name, id string) error {
	_, err := c.c.Prune(ctx, name, id, false)
	if err != nil {
		return err
	}
	return err
}

func (c *remoteCache) NewUpdate(upd *sdcpb.Update) (*Update, error) {
	b, err := proto.Marshal(upd.GetValue())
	if err != nil {
		return nil, err
	}
	rupd := &Update{
		path:  utils.ToStrings(upd.GetPath(), false, false),
		value: b,
	}
	return rupd, nil
}

func (c *remoteCache) Close() error {
	return c.c.Close()
}
