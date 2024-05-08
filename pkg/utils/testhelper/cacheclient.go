package testhelper

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sdcio/cache/proto/cachepb"
	"github.com/sdcio/data-server/mocks/mockcacheclient"
	"github.com/sdcio/data-server/pkg/cache"
	"github.com/sdcio/data-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
)

func ConfigureCacheClientMock(t *testing.T, cacheClient *mockcacheclient.MockClient, updatesIntended []*cache.Update, expectedModify []*cache.Update, expectedDeletes [][]string) {

	// mock the .GetIntendedKeysMeta() call
	cacheClient.EXPECT().GetIntendedKeysMeta(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(

		func(_ context.Context, datastoreName string) (chan *cache.Update, error) {
			rsCh := make(chan *cache.Update)
			go func() {
				for _, u := range updatesIntended {
					rsCh <- u
				}
				close(rsCh)
			}()
			return rsCh, nil
		},
	)

	// mock the .Read() call
	// prepare a map of the updates indexed by the path for quick lookup
	updatesMap := map[string][]*cache.Update{}
	// fill the map
	for _, u := range updatesIntended {
		key := strings.Join(u.GetPath(), pathSep)
		updatesMap[key] = append(updatesMap[key], u)
	}
	cacheClient.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, datastoreName string, opts *cache.Opts, paths [][]string, period time.Duration) []*cache.Update {
			result := make([]*cache.Update, 0, len(paths))
			for _, p := range paths {
				if val, exists := updatesMap[strings.Join(p, pathSep)]; exists {
					result = append(result, val...)
				}
			}
			return result
		},
	)

	// mock the .HasCandidate() call
	cacheClient.EXPECT().HasCandidate(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, _ string, _ string) (bool, error) {
			return true, nil
		},
	)

	// mock the NewUpdate() call
	cacheClient.EXPECT().NewUpdate(gomock.Any()).AnyTimes().DoAndReturn(

		func(upd *sdcpb.Update) (*cache.Update, error) {
			b, err := proto.Marshal(upd.Value)
			if err != nil {
				return nil, err
			}
			return cache.NewUpdate(utils.ToStrings(upd.GetPath(), false, false), b, 0, "", 0), nil

		},
	)

	// mock the .Modify() call
	cacheClient.EXPECT().Modify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, name string, opts *cache.Opts, dels [][]string, upds []*cache.Update) error {
			if opts.Store == cachepb.Store_INTENDED {
				if diff := DiffCacheUpdates(expectedModify, upds); diff != "" {
					t.Errorf("cache.Modify() updates mismatch (-want +got):\n%s", diff)
				}

				if diff := DiffDoubleStringPathSlice(expectedDeletes, dels); diff != "" {
					t.Errorf("cache.Modify() deletes mismatch (-want +got):\n%s", diff)
				}
			}
			return nil
		},
	)
}
