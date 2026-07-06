package node

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
)

type recordingCore struct {
	mu         sync.Mutex
	addNode    int
	addUsers   [][]panel.UserInfo
	delUsers   [][]panel.UserInfo
	deleteNode int
}

func (c *recordingCore) Start() error { return nil }
func (c *recordingCore) Close() error { return nil }
func (c *recordingCore) AddNode(_ string, _ *panel.NodeInfo, _ *conf.Options) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addNode++
	return nil
}
func (c *recordingCore) DelNode(_ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleteNode++
	return nil
}
func (c *recordingCore) AddUsers(p *vCore.AddUsersParams) (int, error) {
	users := append([]panel.UserInfo(nil), p.Users...)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addUsers = append(c.addUsers, users)
	return len(users), nil
}
func (c *recordingCore) GetUserTrafficSlice(_ string, _ bool) ([]panel.UserTraffic, error) {
	return nil, nil
}
func (c *recordingCore) DelUsers(users []panel.UserInfo, _ string, _ *panel.NodeInfo) error {
	deleted := append([]panel.UserInfo(nil), users...)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delUsers = append(c.delUsers, deleted)
	return nil
}
func (c *recordingCore) Protocols() []string { return []string{"vless"} }
func (c *recordingCore) Type() string        { return "sing" }

type userSnapshotServer struct {
	server     *httptest.Server
	mu         sync.Mutex
	serverName string
	users      []panel.UserInfo
	userStatus int
}

func newUserSnapshotServer(t *testing.T, users []panel.UserInfo) *userSnapshotServer {
	t.Helper()
	fixture := &userSnapshotServer{
		serverName: "example.invalid",
		users:      snapshotUsers(users),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (s *userSnapshotServer) close() {
	s.server.Close()
}

func (s *userSnapshotServer) setUsers(users []panel.UserInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = snapshotUsers(users)
	s.userStatus = 0
}

func (s *userSnapshotServer) setUserStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userStatus = status
}

func (s *userSnapshotServer) setServerName(serverName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverName = serverName
}

func (s *userSnapshotServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/v1/server/UniProxy/config":
		s.mu.Lock()
		serverName := s.serverName
		s.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{
			"host":"0.0.0.0",
			"server_port":443,
			"server_name":%q,
			"routes":[],
			"base_config":{"push_interval":3600,"pull_interval":3600},
			"tls":0,
			"network":"tcp",
			"network_settings":{},
			"encryption":"none",
			"encryption_settings":{}
		}`, serverName)
	case "/api/v1/server/UniProxy/user":
		s.mu.Lock()
		status := s.userStatus
		users := snapshotUsers(s.users)
		s.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if len(users) == 0 {
			_, _ = w.Write([]byte(`{"users":[]}`))
			return
		}
		usersJSON, err := json.Marshal(users)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"users":`))
		_, _ = w.Write(usersJSON)
		_, _ = w.Write([]byte(`}`))
	case "/api/v1/server/UniProxy/alivelist":
		_, _ = w.Write([]byte(`{"alive":{}}`))
	default:
		http.NotFound(w, r)
	}
}

func snapshotUsers(users []panel.UserInfo) []panel.UserInfo {
	if users == nil {
		return make([]panel.UserInfo, 0)
	}
	return append([]panel.UserInfo(nil), users...)
}

func newControllerForUserSnapshotTest(t *testing.T, apiHost string, core vCore.Core) *Controller {
	t.Helper()
	client, err := panel.New(&conf.ApiConfig{
		APIHost:  apiHost,
		Key:      "test-token",
		NodeID:   1,
		NodeType: "vless",
		Timeout:  1,
	})
	if err != nil {
		t.Fatalf("panel.New() error = %v", err)
	}
	return NewController(core, client, &conf.Options{})
}

func TestControllerStartAllowsEmptyAuthoritativeUserSnapshot(t *testing.T) {
	limiter.Init()
	fixture := newUserSnapshotServer(t, nil)
	defer fixture.close()
	core := &recordingCore{}
	controller := newControllerForUserSnapshotTest(t, fixture.server.URL, core)
	defer func() { _ = controller.Close() }()

	if err := controller.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if core.addNode != 1 {
		t.Fatalf("AddNode calls = %d, want 1", core.addNode)
	}
	if len(core.addUsers) != 0 {
		t.Fatalf("AddUsers calls = %d, want 0 for empty authorization snapshot", len(core.addUsers))
	}
}

func TestControllerStartRejectsNoChangeUserSnapshotWithoutPriorState(t *testing.T) {
	limiter.Init()
	fixture := newUserSnapshotServer(t, nil)
	defer fixture.close()
	fixture.setUserStatus(http.StatusNotModified)
	core := &recordingCore{}
	controller := newControllerForUserSnapshotTest(t, fixture.server.URL, core)
	defer func() { _ = controller.Close() }()

	if err := controller.Start(); err == nil {
		t.Fatal("Start() succeeded with cold-start no-change user snapshot, want error")
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if core.addNode != 0 {
		t.Fatalf("AddNode calls = %d, want 0 after cold-start no-change user snapshot", core.addNode)
	}
	if len(core.addUsers) != 0 {
		t.Fatalf("AddUsers calls = %d, want 0 after rejected startup", len(core.addUsers))
	}
}

func TestControllerMonitorDeletesUsersWhenAuthoritativeSnapshotBecomesEmpty(t *testing.T) {
	limiter.Init()
	activeUser := panel.UserInfo{Id: 7, Uuid: "bc51a1d0-cb83-448a-9aa3-8d336c626467"}
	fixture := newUserSnapshotServer(t, []panel.UserInfo{activeUser})
	defer fixture.close()
	core := &recordingCore{}
	controller := newControllerForUserSnapshotTest(t, fixture.server.URL, core)
	defer func() { _ = controller.Close() }()

	if err := controller.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fixture.setUsers(nil)
	if err := controller.nodeInfoMonitor(); err != nil {
		t.Fatalf("nodeInfoMonitor() error = %v", err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.addUsers) != 1 || len(core.addUsers[0]) != 1 {
		t.Fatalf("AddUsers batches = %#v, want one initial user", core.addUsers)
	}
	if len(core.delUsers) != 1 || len(core.delUsers[0]) != 1 {
		t.Fatalf("DelUsers batches = %#v, want one deleted user after empty snapshot", core.delUsers)
	}
	if got := core.delUsers[0][0].Uuid; got != activeUser.Uuid {
		t.Fatalf("deleted UUID = %q, want %q", got, activeUser.Uuid)
	}
}

func TestControllerReloadDeletesUsersWhenNodeChangesAndSnapshotBecomesEmpty(t *testing.T) {
	limiter.Init()
	activeUser := panel.UserInfo{Id: 7, Uuid: "bc51a1d0-cb83-448a-9aa3-8d336c626467"}
	fixture := newUserSnapshotServer(t, []panel.UserInfo{activeUser})
	defer fixture.close()
	core := &recordingCore{}
	controller := newControllerForUserSnapshotTest(t, fixture.server.URL, core)
	defer func() { _ = controller.Close() }()

	if err := controller.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fixture.setUsers(nil)
	fixture.setServerName("changed.example.invalid")
	if err := controller.nodeInfoMonitor(); err != nil {
		t.Fatalf("nodeInfoMonitor() error = %v", err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.addUsers) != 1 || len(core.addUsers[0]) != 1 {
		t.Fatalf("AddUsers batches = %#v, want only the initial user add", core.addUsers)
	}
	if len(core.delUsers) != 1 || len(core.delUsers[0]) != 1 {
		t.Fatalf("DelUsers batches = %#v, want one deleted user before reload", core.delUsers)
	}
	if got := core.delUsers[0][0].Uuid; got != activeUser.Uuid {
		t.Fatalf("deleted UUID = %q, want %q", got, activeUser.Uuid)
	}
	if core.deleteNode != 1 {
		t.Fatalf("DelNode calls = %d, want 1", core.deleteNode)
	}
	if core.addNode != 2 {
		t.Fatalf("AddNode calls = %d, want initial add plus reload add", core.addNode)
	}
}
