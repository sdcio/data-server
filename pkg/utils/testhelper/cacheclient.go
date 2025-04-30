package testhelper

// func ConfigureCacheClientMock(t *testing.T, cacheClient *mockcacheclient.MockClient, updatesIntended []*cache.Update, updatesRunning []*cache.Update, expectedModify []*cache.Update, expectedDeletes [][]string) {

// 	cacheClient.EXPECT().InstanceIntentGet(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
// 		func (ctx context.Context, )  {

// 		}

// 	)

// 	Read(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
// 		func(_ context.Context, datastoreName string, opts *cache.Opts, paths [][]string, period time.Duration) []*cache.Update {
// 			if len(paths) == 1 && len(paths[0]) == 0 {
// 				return updatesRunning
// 			}
// 			result := make([]*cache.Update, 0, len(paths))
// 			for _, p := range paths {
// 				if val, exists := updatesMap[opts.Store][strings.Join(p, pathSep)]; exists {
// 					result = append(result, val...)
// 				}
// 			}
// 			return result
// 		},
// 	)

// 	// mock the .HasCandidate() call
// 	cacheClient.EXPECT().HasCandidate(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
// 		func(_ context.Context, _ string, _ string) (bool, error) {
// 			return true, nil
// 		},
// 	)

// 	// mock the NewUpdate() call
// 	cacheClient.EXPECT().NewUpdate(gomock.Any()).AnyTimes().DoAndReturn(

// 		func(upd *sdcpb.Update) (*cache.Update, error) {
// 			b, err := proto.Marshal(upd.Value)
// 			if err != nil {
// 				return nil, err
// 			}
// 			return cache.NewUpdate(utils.ToStrings(upd.GetPath(), false, false), b, 0, "", 0), nil

// 		},
// 	)

// 	// mock the .Modify() call
// 	cacheClient.EXPECT().Modify(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
// 		func(ctx context.Context, name string, opts *cache.Opts, dels [][]string, upds []*cache.Update) error {
// 			if opts.Store == cachepb.Store_INTENDED {
// 				if diff := DiffCacheUpdates(expectedModify, upds); diff != "" {
// 					t.Errorf("cache.Modify() updates mismatch (-want +got):\n%s", diff)
// 				}

// 				if diff := DiffDoubleStringPathSlice(expectedDeletes, dels); diff != "" {
// 					t.Errorf("cache.Modify() deletes mismatch (-want +got):\n%s", diff)
// 				}
// 			}
// 			return nil
// 		},
// 	)
// }
