package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
)

type lifecycleClock struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *lifecycleClock) current() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *lifecycleClock) set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

type lifecycleServer struct {
	*httptest.Server
	store    *sqlite.Store
	accounts *usecase.Accounts
}

func openLifecycleServer(t *testing.T, path string, clock *lifecycleClock) *lifecycleServer {
	t.Helper()
	store, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	accounts, err := usecase.NewAccounts(store, &httpTestHasher{}, clock.current)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(usecase.New(store, clock.current, zerolog.Nop()), staticFiles(), Options{Accounts: accounts}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &lifecycleServer{Server: srv, store: store, accounts: accounts}
}

func (s *lifecycleServer) close(t *testing.T) {
	t.Helper()
	s.Close()
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
}

type lifecycleClient struct {
	client  *http.Client
	binding string
	cookie  *http.Cookie
}

func newLifecycleClient(t *testing.T) *lifecycleClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	return &lifecycleClient{client: client}
}

type lifecycleResponse struct {
	status  int
	header  http.Header
	body    string
	cookies []*http.Cookie
}

func (c *lifecycleClient) request(t *testing.T, baseURL, method, path, body string) lifecycleResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Content-Type", "application/json")
	}
	if path != "/api/session" && path != "/api/login" {
		req.Header.Set("X-Steady-Session", c.binding)
	}
	response, err := c.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return lifecycleResponse{status: response.StatusCode, header: response.Header, body: strings.TrimSpace(string(data)), cookies: response.Cookies()}
}

func assertLifecycleResponse(t *testing.T, response lifecycleResponse, status int, body string) {
	t.Helper()
	if response.status != status || response.body != body {
		t.Fatalf("response = %d %q, want %d %q", response.status, response.body, status, body)
	}
	if body != "" && response.header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", response.header.Get("Content-Type"))
	}
}

func (c *lifecycleClient) discover(t *testing.T, baseURL, username string) {
	t.Helper()
	response := c.request(t, baseURL, http.MethodGet, "/api/session", "")
	if response.status != http.StatusOK || response.header.Get("Content-Type") != "application/json" {
		t.Fatalf("session discovery = %d %q", response.status, response.body)
	}
	var session struct{ Username, Binding string }
	if err := json.Unmarshal([]byte(response.body), &session); err != nil {
		t.Fatal(err)
	}
	if session.Username != username || session.Binding == "" {
		t.Fatal("session discovery returned an unexpected account or empty binding")
	}
	if c.binding != "" && session.Binding != c.binding {
		t.Fatal("persisted session binding changed")
	}
	if len(response.header.Values("Set-Cookie")) != 0 {
		t.Fatal("session discovery renewed the cookie")
	}
	c.binding = session.Binding
}

func (c *lifecycleClient) login(t *testing.T, baseURL, username string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": username + " synthetic password"})
	if err != nil {
		t.Fatal(err)
	}
	response := c.request(t, baseURL, http.MethodPost, "/api/login", string(body))
	assertLifecycleResponse(t, response, http.StatusNoContent, "")
	if len(response.cookies) != 1 || response.cookies[0].Name != "steady_session" {
		t.Fatal("login did not issue exactly one session cookie")
	}
	c.cookie = response.cookies[0]
	c.discover(t, baseURL, username)
}

func createLifecycleAccount(t *testing.T, srv *lifecycleServer, username string) {
	t.Helper()
	if err := srv.accounts.Create(t.Context(), username, []byte(username+" synthetic password")); err != nil {
		t.Fatal(err)
	}
}

func replayLifecycleCookie(t *testing.T, baseURL string, cookie *http.Cookie) {
	t.Helper()
	client := newLifecycleClient(t)
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Replay the original opaque value even if logout removed it from the original jar.
	client.client.Jar.SetCookies(endpoint, []*http.Cookie{{Name: cookie.Name, Value: cookie.Value, Path: "/"}})
	response := client.request(t, baseURL, http.MethodGet, "/api/session", "")
	assertLifecycleResponse(t, response, http.StatusUnauthorized, `{"error":"authentication required"}`)
}

func TestHTTPSessionSurvivesRestartUntilAbsoluteExpiry(t *testing.T) {
	start := fixedNow()
	clock := &lifecycleClock{now: start}
	path := filepath.Join(t.TempDir(), "steady.db")
	srv := openLifecycleServer(t, path, clock)
	createLifecycleAccount(t, srv, "alice")
	client := newLifecycleClient(t)
	client.login(t, srv.URL, "alice")
	if client.cookie.MaxAge != 30*24*60*60 || !client.cookie.Expires.Equal(start.Add(30*24*time.Hour)) {
		t.Fatal("login cookie did not use an absolute 30-day expiry")
	}
	savedCookie, savedBinding := client.cookie.Value, client.binding
	assertLifecycleResponse(t, client.request(t, srv.URL, http.MethodPost, "/api/habits", createBody(idA, "Read")), http.StatusCreated, habitJSON(idA, "Read"))
	srv.close(t)

	clock.set(start.Add(29 * 24 * time.Hour))
	srv = openLifecycleServer(t, path, clock)
	client.discover(t, srv.URL, "alice")
	response := client.request(t, srv.URL, http.MethodGet, "/api/state", "")
	assertLifecycleResponse(t, response, http.StatusOK, stateJSON("week", defaultAccent, habitJSON(idA, "Read")))
	if len(response.header.Values("Set-Cookie")) != 0 {
		t.Fatal("state read renewed the cookie")
	}
	response = client.request(t, srv.URL, http.MethodPut, "/api/habits/"+idA+"/days/2026-11-01", "")
	assertLifecycleResponse(t, response, http.StatusNoContent, "")
	if len(response.header.Values("Set-Cookie")) != 0 {
		t.Fatal("private write renewed the cookie")
	}
	endpoint, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := client.client.Jar.Cookies(endpoint)
	if len(cookies) != 1 || cookies[0].Value != savedCookie || client.binding != savedBinding {
		t.Fatal("restart replaced the original cookie or binding")
	}
	missingBinding := client.binding
	client.binding = ""
	assertLifecycleResponse(t, client.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusUnauthorized, `{"error":"authentication required"}`)
	client.binding = missingBinding
	srv.close(t)

	srv = openLifecycleServer(t, path, clock)
	clock.set(start.Add(30*24*time.Hour - time.Nanosecond))
	client.discover(t, srv.URL, "alice")
	clock.set(start.Add(30 * 24 * time.Hour))
	replayLifecycleCookie(t, srv.URL, client.cookie)
	assertLifecycleResponse(t, client.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusUnauthorized, `{"error":"authentication required"}`)
}

func TestHTTPRevokedCookiesStayRevokedAfterRestart(t *testing.T) {
	for _, action := range []string{"logout", "reset password"} {
		t.Run(action, func(t *testing.T) {
			clock := &lifecycleClock{now: fixedNow()}
			path := filepath.Join(t.TempDir(), "steady.db")
			srv := openLifecycleServer(t, path, clock)
			createLifecycleAccount(t, srv, "alice")
			createLifecycleAccount(t, srv, "bobby")
			first, second, other := newLifecycleClient(t), newLifecycleClient(t), newLifecycleClient(t)
			first.login(t, srv.URL, "alice")
			second.login(t, srv.URL, "alice")
			other.login(t, srv.URL, "bobby")
			if action == "logout" {
				assertLifecycleResponse(t, first.request(t, srv.URL, http.MethodPost, "/api/logout", ""), http.StatusNoContent, "")
			} else if err := srv.accounts.ResetPassword(t.Context(), "alice", []byte("changed synthetic password")); err != nil {
				t.Fatal(err)
			}
			replayLifecycleCookie(t, srv.URL, first.cookie)
			srv.close(t)
			srv = openLifecycleServer(t, path, clock)
			replayLifecycleCookie(t, srv.URL, first.cookie)
			if action == "reset password" {
				replayLifecycleCookie(t, srv.URL, second.cookie)
			} else {
				second.discover(t, srv.URL, "alice")
				assertLifecycleResponse(t, second.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusOK, emptyState)
			}
			other.discover(t, srv.URL, "bobby")
			assertLifecycleResponse(t, other.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusOK, emptyState)
		})
	}
}

func TestHTTPIndependentClientsConvergeAndKeepAccountsIsolated(t *testing.T) {
	srv := openLifecycleServer(t, filepath.Join(t.TempDir(), "steady.db"), &lifecycleClock{now: fixedNow()})
	createLifecycleAccount(t, srv, "alice")
	createLifecycleAccount(t, srv, "bobby")
	first, second, other := newLifecycleClient(t), newLifecycleClient(t), newLifecycleClient(t)
	first.login(t, srv.URL, "alice")
	second.login(t, srv.URL, "alice")
	other.login(t, srv.URL, "bobby")
	if first.cookie.Value == second.cookie.Value || first.binding == second.binding {
		t.Fatal("independent logins reused a session")
	}
	assertLifecycleResponse(t, first.request(t, srv.URL, http.MethodPost, "/api/habits", createBody(idA, "Read")), http.StatusCreated, habitJSON(idA, "Read"))
	assertLifecycleResponse(t, other.request(t, srv.URL, http.MethodPost, "/api/habits", createBody(idB, "Walk")), http.StatusCreated, habitJSON(idB, "Walk"))
	dayPath := "/api/habits/" + idA + "/days/2026-11-01"
	steps := []struct {
		client *lifecycleClient
		method string
		done   []string
	}{
		{first, http.MethodPut, []string{"2026-11-01"}},
		{second, http.MethodDelete, nil},
		{second, http.MethodPut, []string{"2026-11-01"}},
		{first, http.MethodDelete, nil},
	}
	for _, step := range steps {
		assertLifecycleResponse(t, step.client.request(t, srv.URL, step.method, dayPath, ""), http.StatusNoContent, "")
		for _, client := range []*lifecycleClient{first, second} {
			assertLifecycleResponse(t, client.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusOK, stateJSON("week", defaultAccent, habitJSON(idA, "Read", step.done...)))
		}
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			assertLifecycleResponse(t, other.request(t, srv.URL, method, dayPath, ""), http.StatusNotFound, `{"error":"habit not found"}`)
		}
		assertLifecycleResponse(t, first.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusOK, stateJSON("week", defaultAccent, habitJSON(idA, "Read", step.done...)))
		assertLifecycleResponse(t, other.request(t, srv.URL, http.MethodGet, "/api/state", ""), http.StatusOK, stateJSON("week", defaultAccent, habitJSON(idB, "Walk")))
	}
}
