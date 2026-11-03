package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/usecase"
)

type httpTestHasher struct{}

func (*httpTestHasher) Hash(p []byte) (string, error)  { return "test:" + string(p), nil }
func (*httpTestHasher) Verify(h string, p []byte) bool { return h == "test:"+string(p) }
func authLogin(t *testing.T, s *server, username, password string) *http.Cookie {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": username, "password": password})
	r := s.raw(request(t, http.MethodPost, "/api/login", string(b)))
	assertResponse(t, r, 204, "")
	c := r.Result().Cookies()
	if len(c) != 1 {
		t.Fatal("missing session cookie")
	}
	return c[0]
}
func boundRequest(t *testing.T, c *http.Cookie, method, path, body string) *http.Request {
	t.Helper()
	r := request(t, method, path, body)
	r.AddCookie(c)
	sum := sha256.Sum256([]byte("steady-request:" + c.Value))
	r.Header.Set("X-Steady-Session", hex.EncodeToString(sum[:]))
	return r
}

func TestUnauthenticatedEveryPrivateRoute(t *testing.T) {
	s := newServer(t, Options{})
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/state", ""}, {"POST", "/api/habits", createBody(idA, "Read")}, {"PATCH", "/api/habits/" + idA, `{"name":"changed"}`}, {"DELETE", "/api/habits/" + idA, ""}, {"POST", "/api/habits/" + idA + "/restore", ""}, {"PUT", "/api/habits/" + idA + "/days/2026-11-01", ""}, {"DELETE", "/api/habits/" + idA + "/days/2026-11-01", ""}, {"PUT", "/api/settings", `{"view":"year"}`}} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			assertResponse(t, s.raw(request(t, tc.method, tc.path, tc.body)), 401, `{"error":"authentication required"}`)
		})
	}
	assertResponse(t, s.do(request(t, "GET", "/api/state", "")), 200, emptyState)
	assertResponse(t, s.raw(request(t, "GET", "/healthz", "")), 200, `{"status":"ok"}`)
}

func TestLoginCookieSessionBindingLogout(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(fmt.Sprint(secure), func(t *testing.T) {
			s := newServer(t, Options{CookieSecure: secure})
			cookie := authLogin(t, s, "alice", "alice password")
			if cookie.Name != "steady_session" || cookie.Path != "/" || !cookie.HttpOnly || cookie.Secure != secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" || cookie.MaxAge != 30*24*60*60 || !cookie.Expires.Equal(fixedNow().Add(usecase.SessionLifetime)) {
				t.Fatal("cookie attributes violate contract")
			}
			r := request(t, "GET", "/api/session", "")
			r.AddCookie(cookie)
			rec := s.raw(r)
			if rec.Code != 200 {
				t.Fatal(rec.Code)
			}
			var session struct{ Username, Binding string }
			if e := json.Unmarshal(rec.Body.Bytes(), &session); e != nil {
				t.Fatal(e)
			}
			sum := sha256.Sum256([]byte("steady-request:" + cookie.Value))
			if session.Username != "alice" || session.Binding != hex.EncodeToString(sum[:]) {
				t.Fatal("session binding")
			}
			for _, binding := range []string{"", strings.Repeat("0", 64)} {
				r = boundRequest(t, cookie, "GET", "/api/state", "")
				r.Header.Set("X-Steady-Session", binding)
				assertResponse(t, s.raw(r), 401, `{"error":"authentication required"}`)
			}
			assertResponse(t, s.raw(boundRequest(t, cookie, "POST", "/api/logout", "")), 204, "")
			assertResponse(t, s.raw(boundRequest(t, cookie, "GET", "/api/state", "")), 401, `{"error":"authentication required"}`)
			rec = s.raw(request(t, "POST", "/api/logout", ""))
			assertResponse(t, rec, 204, "")
			expired := rec.Result().Cookies()
			if len(expired) != 1 || expired[0].MaxAge != -1 || expired[0].Secure != secure || !expired[0].HttpOnly || expired[0].SameSite != http.SameSiteStrictMode || expired[0].Path != "/" || !expired[0].Expires.Before(fixedNow()) {
				t.Fatal("expiration cookie attributes")
			}
		})
	}
}

func TestLoginRejectsBadBodiesAndUsesGenericErrors(t *testing.T) {
	for _, tc := range []struct {
		body    string
		status  int
		message string
	}{{`{`, 400, "invalid JSON body"}, {`{"username":"alice","password":"wrong password"}`, 401, "invalid credentials"}, {`{"username":"unknown","password":"wrong password"}`, 401, "invalid credentials"}, {`{"username":"alice","password":"alice password","user_id":"forged"}`, 400, "invalid JSON body"}, {`{"username":"alice","password":"` + strings.Repeat("x", 5000) + `"}`, 400, "invalid JSON body"}, {`{"username":"alice","password":"alice password"}{}`, 400, "invalid JSON body"}} {
		t.Run(fmt.Sprint(tc.status)+tc.message+fmt.Sprint(len(tc.body)), func(t *testing.T) {
			s := newServer(t, Options{})
			r := s.raw(request(t, "POST", "/api/login", tc.body))
			assertResponse(t, r, tc.status, `{"error":"`+tc.message+`"}`)
			if len(r.Result().Cookies()) != 0 {
				t.Fatal("rejected login set cookie")
			}
		})
	}
	var logs bytes.Buffer
	s := newLoggedServer(t, Options{}, zerolog.New(&logs))
	_ = s.store.Close()
	r := s.raw(request(t, "POST", "/api/login", `{"username":"alice","password":"alice password"}`))
	assertResponse(t, r, 500, `{"error":"internal error"}`)
	for _, secret := range []string{"alice", "password", "test:", s.cookie.Value, "database is closed"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("auth log disclosed secret or raw error")
		}
	}
}

func TestCookieReplacementRejectsOldBindingAndForeignMutations(t *testing.T) {
	s := newServer(t, Options{})
	alice := s.cookie
	assertResponse(t, s.raw(boundRequest(t, alice, "POST", "/api/habits", createBody(idA, "Private"))), 201, habitJSON(idA, "Private"))
	assertResponse(t, s.raw(boundRequest(t, alice, "PUT", "/api/habits/"+idA+"/days/2026-11-01", "")), 204, "")
	if e := s.accounts.Create(t.Context(), "bobby", []byte("bobby password")); e != nil {
		t.Fatal(e)
	}
	bob := authLogin(t, s, "bobby", "bobby password")
	stale := boundRequest(t, bob, "POST", "/api/habits", createBody(idB, "stale"))
	stale.Header.Set("X-Steady-Session", s.binding)
	assertResponse(t, s.raw(stale), 401, `{"error":"authentication required"}`)
	for _, tc := range []struct{ method, path, body string }{{"PATCH", "/api/habits/" + idA, `{"name":"stolen"}`}, {"DELETE", "/api/habits/" + idA, ""}, {"PUT", "/api/habits/" + idA + "/days/2026-11-01", ""}, {"DELETE", "/api/habits/" + idA + "/days/2026-11-01", ""}, {"POST", "/api/habits/" + idA + "/restore", ""}} {
		assertResponse(t, s.raw(boundRequest(t, bob, tc.method, tc.path, tc.body)), 404, `{"error":"habit not found"}`)
	}
	assertResponse(t, s.raw(boundRequest(t, alice, "DELETE", "/api/habits/"+idA, "")), 204, "")
	assertResponse(t, s.raw(boundRequest(t, bob, "POST", "/api/habits/"+idA+"/restore", "")), 404, `{"error":"habit not found"}`)
	assertResponse(t, s.raw(boundRequest(t, alice, "POST", "/api/habits/"+idA+"/restore", "")), 204, "")
	assertResponse(t, s.raw(boundRequest(t, bob, "PUT", "/api/settings", `{"view":"year","accent":"#5B8CEB"}`)), 204, "")
	assertResponse(t, s.raw(boundRequest(t, bob, "GET", "/api/state", "")), 200, stateJSON("year", "#5B8CEB"))
	assertResponse(t, s.raw(boundRequest(t, alice, "GET", "/api/state", "")), 200, stateJSON("week", defaultAccent, habitJSON(idA, "Private", "2026-11-01")))
	for _, path := range []string{"/api/habits", "/api/settings"} {
		method := "POST"
		body := createBody(idB, "forged")
		if path == "/api/settings" {
			method = "PUT"
			body = `{"view":"year"}`
		}
		body = strings.TrimSuffix(body, "}") + `,"user_id":"` + s.ownerID + `"}`
		assertResponse(t, s.raw(boundRequest(t, bob, method, path, body)), 400, `{"error":"invalid JSON body"}`)
	}
}

func TestAuthHostAndCrossOriginHaveNoEffects(t *testing.T) {
	for _, path := range []string{"/api/login", "/api/logout"} {
		for _, badHost := range []bool{false, true} {
			s := newServer(t, Options{})
			r := boundRequest(t, s.cookie, "POST", path, `{"username":"alice","password":"alice password"}`)
			message := "cross-origin request rejected"
			r.Header.Set("Sec-Fetch-Site", "cross-site")
			if badHost {
				r.Host = "evil.example"
				message = "host not allowed"
			}
			rec := s.raw(r)
			assertResponse(t, rec, 403, `{"error":"`+message+`"}`)
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("rejected auth changed cookie")
			}
			if _, e := s.accounts.Current(t.Context(), s.cookie.Value); e != nil {
				t.Fatal("rejected logout revoked session")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		}
	}
}

func TestLoginLimiterBoundsAndExpiry(t *testing.T) {
	now := fixedNow()
	l := newLoginLimiter(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if !l.allow("192.0.2.1:123") {
			t.Fatal("early IP limit")
		}
	}
	if l.allow("192.0.2.1:456") {
		t.Fatal("IP quota ignores ports")
	}
	now = now.Add(15 * time.Minute)
	if !l.allow("192.0.2.1:456") {
		t.Fatal("IP window did not expire")
	}
	l = newLoginLimiter(func() time.Time { return now })
	for i := 0; i < 30; i++ {
		if !l.allow(fmt.Sprintf("192.0.2.%d:1", i)) {
			t.Fatal("early global limit")
		}
	}
	if l.allow("192.0.2.100:1") {
		t.Fatal("global quota")
	}
	now = now.Add(time.Minute)
	if !l.allow("192.0.2.100:1") {
		t.Fatal("global window did not expire")
	}
	l = newLoginLimiter(func() time.Time { return now })
	for i := 0; i < 1024; i++ {
		l.ips[fmt.Sprintf("198.51.%d.%d", i/256, i%256)] = loginWindow{started: now, attempts: 1}
	}
	if l.allow("203.0.113.1:1") {
		t.Fatal("full IP table did not fail closed")
	}
	now = now.Add(15 * time.Minute)
	if !l.allow("203.0.113.1:1") || len(l.ips) != 1 {
		t.Fatal("expired table entries were not pruned")
	}
}

func TestLoginLimiterConcurrentVerificationAndForwardedHeaderIgnored(t *testing.T) {
	l := newLoginLimiter(fixedNow)
	first, second, third := l.acquire(), l.acquire(), l.acquire()
	if !first || !second || third {
		t.Fatal("verification semaphore bound")
	}
	l.release()
	if !l.acquire() {
		t.Fatal("slot not released")
	}
	l.release()
	l.release()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.acquire() {
				time.Sleep(time.Millisecond)
				l.release()
			}
		}()
	}
	wg.Wait()
	s := newServer(t, Options{})
	for i := 0; i < 6; i++ {
		r := request(t, "POST", "/api/login", `{"username":"alice","password":"alice password"}`)
		r.RemoteAddr = "192.0.2.10:123"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
		rec := s.raw(r)
		if i < 5 {
			assertResponse(t, rec, 204, "")
		} else {
			assertResponse(t, rec, 429, `{"error":"too many login attempts"}`)
		}
	}
}

func TestNewRequiresAccounts(t *testing.T) {
	if _, e := New(nil, staticFiles(), Options{}, zerolog.Nop()); e == nil {
		t.Fatal("missing Accounts accepted")
	}
}

type blockingHTTPHasher struct {
	started chan struct{}
	release <-chan struct{}
}

func (h *blockingHTTPHasher) Hash(p []byte) (string, error) { return "test:" + string(p), nil }
func (h *blockingHTTPHasher) Verify(hash string, p []byte) bool {
	h.started <- struct{}{}
	<-h.release
	return hash == "test:"+string(p)
}
func TestHTTPRejectsExcessConcurrentVerifications(t *testing.T) {
	s := newServer(t, Options{})
	release := make(chan struct{})
	hasher := &blockingHTTPHasher{started: make(chan struct{}, 2), release: release}
	accounts, e := usecase.NewAccounts(s.store, hasher, fixedNow)
	if e != nil {
		t.Fatal(e)
	}
	handler, e := New(usecase.New(s.store, fixedNow, zerolog.Nop()), staticFiles(), Options{Accounts: accounts}, zerolog.Nop())
	if e != nil {
		t.Fatal(e)
	}
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			r := request(t, "POST", "/api/login", `{"username":"alice","password":"alice password"}`)
			r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1", i)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			results <- rec.Code
		}(i)
	}
	<-hasher.started
	<-hasher.started
	r := request(t, "POST", "/api/login", `{"username":"alice","password":"alice password"}`)
	r.RemoteAddr = "192.0.2.3:1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	assertResponse(t, rec, 429, `{"error":"too many login attempts"}`)
	close(release)
	for i := 0; i < 2; i++ {
		if status := <-results; status != 204 {
			t.Fatal("admitted verification failed")
		}
	}
}
