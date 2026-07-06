package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/InazumaV/V2bX/conf"
	"github.com/vmihailenco/msgpack/v5"
)

func newUserListTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := New(&conf.ApiConfig{
		APIHost:  serverURL,
		Key:      "test-token",
		NodeID:   1,
		NodeType: "vless",
		Timeout:  1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func TestGetUserListTreatsHTTP304AsNoChange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err != nil {
		t.Fatalf("GetUserList() error = %v", err)
	}
	if users != nil {
		t.Fatalf("GetUserList() users = %#v, want nil no-change marker", users)
	}
}

func TestGetUserListTreatsEmptyJSONUsersAsAuthoritativeSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[]}`))
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err != nil {
		t.Fatalf("GetUserList() error = %v", err)
	}
	if users == nil {
		t.Fatal("GetUserList() returned nil for an authoritative empty users snapshot")
	}
	if len(users) != 0 {
		t.Fatalf("GetUserList() returned %d users, want 0", len(users))
	}
}

func TestGetUserListRejectsMissingJSONUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err == nil {
		t.Fatalf("GetUserList() users = %#v, want missing users error", users)
	}
	if !strings.Contains(err.Error(), `expected "users" array`) {
		t.Fatalf("GetUserList() error = %v, want expected users array", err)
	}
}

func TestGetUserListRejectsWrongCaseJSONUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Users":[]}`))
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err == nil {
		t.Fatalf("GetUserList() users = %#v, want wrong-case users error", users)
	}
	if !strings.Contains(err.Error(), `expected "users" array`) {
		t.Fatalf("GetUserList() error = %v, want expected users array", err)
	}
}

func TestGetUserListRejectsDuplicateJSONUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":1,"uuid":"active","speed_limit":0,"device_limit":0}],"users":[]}`))
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err == nil {
		t.Fatalf("GetUserList() users = %#v, want duplicate users error", users)
	}
	if !strings.Contains(err.Error(), `duplicate object member name "users"`) {
		t.Fatalf("GetUserList() error = %v, want duplicate users error", err)
	}
}

func TestGetUserListTreatsEmptyMsgpackUsersAsAuthoritativeSnapshot(t *testing.T) {
	body, err := msgpack.Marshal(&UserListBody{Users: []UserInfo{}})
	if err != nil {
		t.Fatalf("msgpack.Marshal() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-msgpack")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err != nil {
		t.Fatalf("GetUserList() error = %v", err)
	}
	if users == nil {
		t.Fatal("GetUserList() returned nil for an authoritative empty users snapshot")
	}
	if len(users) != 0 {
		t.Fatalf("GetUserList() returned %d users, want 0", len(users))
	}
}

func TestGetUserListRejectsMissingMsgpackUsers(t *testing.T) {
	body, err := msgpack.Marshal(map[string]any{"data": []UserInfo{}})
	if err != nil {
		t.Fatalf("msgpack.Marshal() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-msgpack")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	users, err := newUserListTestClient(t, server.URL).GetUserList()
	if err == nil {
		t.Fatalf("GetUserList() users = %#v, want missing users error", users)
	}
	if !strings.Contains(err.Error(), `expected "users" array`) {
		t.Fatalf("GetUserList() error = %v, want expected users array", err)
	}
}
