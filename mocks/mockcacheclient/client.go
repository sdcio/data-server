// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/cache/cache.go
//
// Generated by this command:
//
//	mockgen -package=mockcacheclient -source=pkg/cache/cache.go -destination=./mocks/mockcacheclient/client.go
//

// Package mockcacheclient is a generated GoMock package.
package mockcacheclient

import (
	context "context"
	reflect "reflect"
	time "time"

	cache "github.com/sdcio/cache/pkg/cache"
	cachepb "github.com/sdcio/cache/proto/cachepb"
	cache0 "github.com/sdcio/data-server/pkg/cache"
	schema_server "github.com/sdcio/sdc-protos/sdcpb"
	gomock "go.uber.org/mock/gomock"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
	isgomock struct{}
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// ApplyPrune mocks base method.
func (m *MockClient) ApplyPrune(ctx context.Context, name, id string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ApplyPrune", ctx, name, id)
	ret0, _ := ret[0].(error)
	return ret0
}

// ApplyPrune indicates an expected call of ApplyPrune.
func (mr *MockClientMockRecorder) ApplyPrune(ctx, name, id any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ApplyPrune", reflect.TypeOf((*MockClient)(nil).ApplyPrune), ctx, name, id)
}

// Clone mocks base method.
func (m *MockClient) Clone(ctx context.Context, name, clone string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Clone", ctx, name, clone)
	ret0, _ := ret[0].(error)
	return ret0
}

// Clone indicates an expected call of Clone.
func (mr *MockClientMockRecorder) Clone(ctx, name, clone any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Clone", reflect.TypeOf((*MockClient)(nil).Clone), ctx, name, clone)
}

// Close mocks base method.
func (m *MockClient) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockClient)(nil).Close))
}

// Commit mocks base method.
func (m *MockClient) Commit(ctx context.Context, name, candidate string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Commit", ctx, name, candidate)
	ret0, _ := ret[0].(error)
	return ret0
}

// Commit indicates an expected call of Commit.
func (mr *MockClientMockRecorder) Commit(ctx, name, candidate any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Commit", reflect.TypeOf((*MockClient)(nil).Commit), ctx, name, candidate)
}

// Create mocks base method.
func (m *MockClient) Create(ctx context.Context, name string, ephemeral, cached bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, name, ephemeral, cached)
	ret0, _ := ret[0].(error)
	return ret0
}

// Create indicates an expected call of Create.
func (mr *MockClientMockRecorder) Create(ctx, name, ephemeral, cached any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockClient)(nil).Create), ctx, name, ephemeral, cached)
}

// CreateCandidate mocks base method.
func (m *MockClient) CreateCandidate(ctx context.Context, name, candidate, owner string, priority int32) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCandidate", ctx, name, candidate, owner, priority)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateCandidate indicates an expected call of CreateCandidate.
func (mr *MockClientMockRecorder) CreateCandidate(ctx, name, candidate, owner, priority any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCandidate", reflect.TypeOf((*MockClient)(nil).CreateCandidate), ctx, name, candidate, owner, priority)
}

// CreatePruneID mocks base method.
func (m *MockClient) CreatePruneID(ctx context.Context, name string, force bool) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePruneID", ctx, name, force)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreatePruneID indicates an expected call of CreatePruneID.
func (mr *MockClientMockRecorder) CreatePruneID(ctx, name, force any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePruneID", reflect.TypeOf((*MockClient)(nil).CreatePruneID), ctx, name, force)
}

// Delete mocks base method.
func (m *MockClient) Delete(ctx context.Context, name string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, name)
	ret0, _ := ret[0].(error)
	return ret0
}

// Delete indicates an expected call of Delete.
func (mr *MockClientMockRecorder) Delete(ctx, name any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockClient)(nil).Delete), ctx, name)
}

// DeleteCandidate mocks base method.
func (m *MockClient) DeleteCandidate(ctx context.Context, name, candidate string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteCandidate", ctx, name, candidate)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteCandidate indicates an expected call of DeleteCandidate.
func (mr *MockClientMockRecorder) DeleteCandidate(ctx, name, candidate any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteCandidate", reflect.TypeOf((*MockClient)(nil).DeleteCandidate), ctx, name, candidate)
}

// Discard mocks base method.
func (m *MockClient) Discard(ctx context.Context, name, candidate string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Discard", ctx, name, candidate)
	ret0, _ := ret[0].(error)
	return ret0
}

// Discard indicates an expected call of Discard.
func (mr *MockClientMockRecorder) Discard(ctx, name, candidate any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Discard", reflect.TypeOf((*MockClient)(nil).Discard), ctx, name, candidate)
}

// Exists mocks base method.
func (m *MockClient) Exists(ctx context.Context, name string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Exists", ctx, name)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Exists indicates an expected call of Exists.
func (mr *MockClientMockRecorder) Exists(ctx, name any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Exists", reflect.TypeOf((*MockClient)(nil).Exists), ctx, name)
}

// GetCandidates mocks base method.
func (m *MockClient) GetCandidates(ctx context.Context, name string) ([]*cache.CandidateDetails, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCandidates", ctx, name)
	ret0, _ := ret[0].([]*cache.CandidateDetails)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCandidates indicates an expected call of GetCandidates.
func (mr *MockClientMockRecorder) GetCandidates(ctx, name any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCandidates", reflect.TypeOf((*MockClient)(nil).GetCandidates), ctx, name)
}

// GetChanges mocks base method.
func (m *MockClient) GetChanges(ctx context.Context, name, candidate string) ([]*cache0.Change, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetChanges", ctx, name, candidate)
	ret0, _ := ret[0].([]*cache0.Change)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetChanges indicates an expected call of GetChanges.
func (mr *MockClientMockRecorder) GetChanges(ctx, name, candidate any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetChanges", reflect.TypeOf((*MockClient)(nil).GetChanges), ctx, name, candidate)
}

// GetKeys mocks base method.
func (m *MockClient) GetKeys(ctx context.Context, name string, store cachepb.Store) (chan *cache0.Update, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetKeys", ctx, name, store)
	ret0, _ := ret[0].(chan *cache0.Update)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetKeys indicates an expected call of GetKeys.
func (mr *MockClientMockRecorder) GetKeys(ctx, name, store any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetKeys", reflect.TypeOf((*MockClient)(nil).GetKeys), ctx, name, store)
}

// HasCandidate mocks base method.
func (m *MockClient) HasCandidate(ctx context.Context, name, candidate string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HasCandidate", ctx, name, candidate)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HasCandidate indicates an expected call of HasCandidate.
func (mr *MockClientMockRecorder) HasCandidate(ctx, name, candidate any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HasCandidate", reflect.TypeOf((*MockClient)(nil).HasCandidate), ctx, name, candidate)
}

// List mocks base method.
func (m *MockClient) List(ctx context.Context) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List", ctx)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// List indicates an expected call of List.
func (mr *MockClientMockRecorder) List(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "List", reflect.TypeOf((*MockClient)(nil).List), ctx)
}

// Modify mocks base method.
func (m *MockClient) Modify(ctx context.Context, name string, opts *cache0.Opts, dels [][]string, upds []*cache0.Update) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Modify", ctx, name, opts, dels, upds)
	ret0, _ := ret[0].(error)
	return ret0
}

// Modify indicates an expected call of Modify.
func (mr *MockClientMockRecorder) Modify(ctx, name, opts, dels, upds any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Modify", reflect.TypeOf((*MockClient)(nil).Modify), ctx, name, opts, dels, upds)
}

// NewUpdate mocks base method.
func (m *MockClient) NewUpdate(arg0 *schema_server.Update) (*cache0.Update, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewUpdate", arg0)
	ret0, _ := ret[0].(*cache0.Update)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewUpdate indicates an expected call of NewUpdate.
func (mr *MockClientMockRecorder) NewUpdate(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewUpdate", reflect.TypeOf((*MockClient)(nil).NewUpdate), arg0)
}

// Read mocks base method.
func (m *MockClient) Read(ctx context.Context, name string, opts *cache0.Opts, paths [][]string, period time.Duration) []*cache0.Update {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Read", ctx, name, opts, paths, period)
	ret0, _ := ret[0].([]*cache0.Update)
	return ret0
}

// Read indicates an expected call of Read.
func (mr *MockClientMockRecorder) Read(ctx, name, opts, paths, period any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Read", reflect.TypeOf((*MockClient)(nil).Read), ctx, name, opts, paths, period)
}

// ReadCh mocks base method.
func (m *MockClient) ReadCh(ctx context.Context, name string, opts *cache0.Opts, paths [][]string, period time.Duration) chan *cache0.Update {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadCh", ctx, name, opts, paths, period)
	ret0, _ := ret[0].(chan *cache0.Update)
	return ret0
}

// ReadCh indicates an expected call of ReadCh.
func (mr *MockClientMockRecorder) ReadCh(ctx, name, opts, paths, period any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadCh", reflect.TypeOf((*MockClient)(nil).ReadCh), ctx, name, opts, paths, period)
}