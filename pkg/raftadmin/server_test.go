package raftadmin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dgate-io/dgate-api/pkg/util/logadapter"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type MockTransport struct {
	mock.Mock
}

var _ raft.Transport = (*MockTransport)(nil)

func (m *MockTransport) Consumer() <-chan raft.RPC {
	args := m.Called()
	return args.Get(0).(chan raft.RPC)
}

func (m *MockTransport) LocalAddr() raft.ServerAddress {
	args := m.Called()
	return args.Get(0).(raft.ServerAddress)
}

func (m *MockTransport) AppendEntries(id raft.ServerID, target raft.ServerAddress, args *raft.AppendEntriesRequest, resp *raft.AppendEntriesResponse) error {
	args2 := m.Called(id, target, args, resp)
	return args2.Error(0)
}

func (m *MockTransport) RequestVote(id raft.ServerID, target raft.ServerAddress, args *raft.RequestVoteRequest, resp *raft.RequestVoteResponse) error {
	args2 := m.Called(id, target, args, resp)
	return args2.Error(0)
}

func (m *MockTransport) InstallSnapshot(id raft.ServerID, target raft.ServerAddress, args *raft.InstallSnapshotRequest, resp *raft.InstallSnapshotResponse, rdr io.Reader) error {
	args2 := m.Called(id, target, args, resp, rdr)
	return args2.Error(0)
}

func (m *MockTransport) AppendEntriesPipeline(id raft.ServerID, target raft.ServerAddress) (raft.AppendPipeline, error) {
	args := m.Called(id, target)
	return args.Get(0).(raft.AppendPipeline), args.Error(1)
}

func (m *MockTransport) EncodePeer(id raft.ServerID, addr raft.ServerAddress) []byte {
	args := m.Called(id, addr)
	return args.Get(0).([]byte)
}

func (m *MockTransport) DecodePeer(b []byte) raft.ServerAddress {
	args := m.Called(b)
	return args.Get(0).(raft.ServerAddress)
}

func (m *MockTransport) SetHeartbeatHandler(h func(raft.RPC)) {
	m.Called(h)
}

func (m *MockTransport) TimeoutNow(id raft.ServerID, target raft.ServerAddress, args *raft.TimeoutNowRequest, resp *raft.TimeoutNowResponse) error {
	args2 := m.Called(id, target, args, resp)
	return args2.Error(0)
}

type MockFSM struct {
	mock.Mock
}

var _ raft.FSM = (*MockFSM)(nil)

func (m *MockFSM) Apply(l *raft.Log) interface{} {
	args := m.Called(l)
	return args.Get(0)
}

func (m *MockFSM) Snapshot() (raft.FSMSnapshot, error) {
	args := m.Called()
	return args.Get(0).(raft.FSMSnapshot), args.Error(1)
}

func (m *MockFSM) Restore(io.ReadCloser) error {
	args := m.Called()
	return args.Error(0)
}

func setupRaftAdmin(t *testing.T) *httptest.Server {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = "1"
	raftConfig.Logger = logadapter.NewZap2HCLogAdapter(zap.NewNop())
	mockFSM := &MockFSM{}
	mockFSM.On("Apply", mock.Anything).Return(nil)

	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()
	snapStore := raft.NewInmemSnapshotStore()

	mocktp := new(MockTransport)
	mocktp.On("LocalAddr").Return(raft.ServerAddress("localhost:9090"))
	mocktp.On("Consumer").Return(make(chan raft.RPC))
	mocktp.On("SetHeartbeatHandler", mock.Anything).Return()
	mocktp.On("EncodePeer", mock.Anything, mock.Anything).Return([]byte{})
	mocktp.On("EncodePeer", mock.Anything).Return(raft.ServerAddress("localhost:9090"))

	raftNode, err := raft.NewRaft(
		raftConfig, mockFSM, logStore,
		stableStore, snapStore, mocktp,
	)
	if err != nil {
		t.Fatal(err)
	}
	err = raftNode.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{
			Suffrage: raft.Voter,
			ID:       "1",
			Address:  raft.ServerAddress("localhost:9090"),
		}},
	}).Error()
	if err != nil {
		t.Fatal(err)
	}
	<-time.After(time.Second * 5)

	raftAdmin := NewServer(
		raftNode, zap.NewNop(),
		[]raft.ServerAddress{
			"localhost:9090",
		},
	)
	mux := http.NewServeMux()
	mux.Handle("/raftadmin/", raftAdmin)
	server := httptest.NewServer(mux)
	return server
}

type raftAdminMockClient struct {
	mock.Mock
	t   *testing.T
	res *http.Response
	Doer
}

func (m *raftAdminMockClient) Do(req *http.Request) (*http.Response, error) {
	m.Called(req)
	return m.res, nil
}

func TestRaft(t *testing.T) {
	server := setupRaftAdmin(t)

	// mock raft.Raft
	mockClient := &raftAdminMockClient{
		t: t, res: &http.Response{
			StatusCode: http.StatusAccepted,
			Body: io.NopCloser(strings.NewReader(
				`{"index": 1}`,
			)),
		},
	}
	mockClient.On("Do", mock.Anything).
		Return(mockClient.res, nil)

	ctx := context.Background()
	client := NewClient(
		server.Client().Do,
		zap.NewNop(), "http",
	)
	serverAddr := raft.ServerAddress(server.Listener.Addr().String())
	leader, err := client.Leader(ctx, serverAddr)
	if err != nil {
		t.Error(err)
		return
	}
	assert.Equal(t, leader.Address, "localhost:9090")
	assert.Equal(t, leader.ID, "1")
}
