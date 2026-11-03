package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
)

const (
	idA = "00000000-0000-4000-8000-00000000000a"
	idB = "00000000-0000-4000-8000-00000000000b"
	idC = "00000000-0000-4000-8000-00000000000c"
	idD = "00000000-0000-4000-8000-00000000000d"
)

const (
	// loopbackHost is the Host a browser on the machine itself sends; allowedHost is the one extra name the
	// standard test server accepts.
	loopbackHost = "127.0.0.1:8770"
	allowedHost  = "steady.example.com"

	defaultAccent = "#4FA872"
	emptyState    = `{"habits":[],"view":"week","accent":"#4FA872"}`
)

// The files of the static filesystem, kept apart from staticFiles so TestStatic can compare against them.
const (
	indexHTML = "<!doctype html><title>t</title>"
	stylesCSS = "body{}"
	mainJS    = "export {};"
)

func staticFiles() fstest.MapFS {
	return fstest.MapFS{
		"index.html":   {Data: []byte(indexHTML)},
		"styles.css":   {Data: []byte(stylesCSS)},
		"dist/main.js": {Data: []byte(mainJS)},
		// In the filesystem but not matched by dist/*.js, so it is never read at startup and must never be served.
		"dist/notes.txt": {Data: []byte("not a script")},
	}
}

func fixedNow() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }

// server is the real stack: a SQLite file, the use case service on it and the HTTP handler on that. Going through
// all three makes a status code the product of validation, storage and mapping together, as a client sees it.
type server struct {
	handler  http.Handler
	store    *sqlite.Store
	accounts *usecase.Accounts
	cookie   *http.Cookie
	binding  string
	ownerID  string
}

func newServer(t *testing.T, opts Options) *server {
	t.Helper()
	return newLoggedServer(t, opts, zerolog.Nop())
}

// newLoggedServer is newServer with the HTTP handler logging to log; the service stays quiet.
func newLoggedServer(t *testing.T, opts Options, log zerolog.Logger) *server {
	t.Helper()
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "steady.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() {
		// A second Close is a no-op, so a test may close the store itself.
		if err := store.Close(); err != nil {
			t.Errorf("store.Close: %v", err)
		}
	})
	svc := usecase.New(store, fixedNow, zerolog.Nop())
	accounts, err := usecase.NewAccounts(store, &httpTestHasher{}, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if err = accounts.Create(t.Context(), "alice", []byte("alice password")); err != nil {
		t.Fatal(err)
	}
	token, _, err := accounts.Login(t.Context(), "alice", []byte("alice password"))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := accounts.Current(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	opts.Accounts = accounts
	h, err := New(svc, staticFiles(), opts, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	digest := sha256.Sum256([]byte("steady-request:" + token))
	return &server{handler: h, store: store, accounts: accounts, cookie: &http.Cookie{Name: "steady_session", Value: token}, binding: hex.EncodeToString(digest[:]), ownerID: owner.ID}
}

func (s *server) do(req *http.Request) *httptest.ResponseRecorder {
	if strings.HasPrefix(req.URL.Path, "/api/") && req.URL.Path != "/api/login" {
		req.AddCookie(s.cookie)
		req.Header.Set("X-Steady-Session", s.binding)
	}
	return s.raw(req)
}

func (s *server) raw(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// request builds what a browser on the loopback address sends: the loopback Host and, for every method that
// changes data, Sec-Fetch-Site: same-origin. A test that needs another Host or header overwrites it.
func request(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, rd)
	req.Host = loopbackHost
	if method != http.MethodGet {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	return req
}

// assertResponse compares the status and the body. Every body this API writes is JSON and a 204 has none, so a
// non-empty wantBody also has to come with Content-Type: application/json.
func assertResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	got := strings.TrimSpace(rec.Body.String())
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, wantStatus, got)
	}
	if got != wantBody {
		t.Errorf("body = %q, want %q", got, wantBody)
	}
	if wantBody == "" {
		return
	}
	if gotType := rec.Header().Get("Content-Type"); gotType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotType, "application/json")
	}
}

func createBody(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `"}`
}

// habitJSON is a habit as the API writes it; done lists its day keys.
func habitJSON(id, name string, done ...string) string {
	days := make([]string, 0, len(done))
	for _, day := range done {
		days = append(days, `"`+day+`"`)
	}
	return `{"id":"` + id + `","name":"` + name + `","done":[` + strings.Join(days, ",") + `]}`
}

// stateJSON is the body of GET /api/state.
func stateJSON(view, accent string, habits ...string) string {
	return `{"habits":[` + strings.Join(habits, ",") + `],"view":"` + view + `","accent":"` + accent + `"}`
}

func TestRoutes(t *testing.T) {
	const (
		invalidID   = `{"error":"invalid habit id: want a lowercase UUID"}`
		invalidJSON = `{"error":"invalid JSON body"}`
		invalidDay  = `{"error":"invalid day: want YYYY-MM-DD"}`
		notFound    = `{"error":"habit not found"}`
	)
	// 60 runes in 120 bytes: a limit counted in bytes would reject it.
	name60 := strings.Repeat("é", 60)
	// Valid JSON in every respect but its size, so only the 4096-byte cap can reject it.
	oversized := createBody(idD, "x")
	oversized += strings.Repeat(" ", 5000-len(oversized))
	habitA := habitJSON(idA, "New")
	habitB := habitJSON(idB, "Untitled")
	habitC := habitJSON(idC, name60)

	// The steps build on each other: they run in order against one server and stop at the first failure.
	steps := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
		statusOnly bool // the body of a 405 is written by net/http and is not part of the API
	}{
		{
			name:       "1 state of a fresh database",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "2 create trims the name",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idA, "  Read  "),
			wantStatus: http.StatusCreated,
			wantBody:   habitJSON(idA, "Read"),
		},
		{
			name:       "3 create with an empty name",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idB, ""),
			wantStatus: http.StatusCreated,
			wantBody:   habitJSON(idB, "Untitled"),
		},
		{
			name:       "4 create with a malformed id",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody("abc", "x"),
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidID,
		},
		{
			name:       "5 create with an uppercase UUID",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody("00000000-0000-4000-8000-00000000000D", "x"),
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidID,
		},
		{
			name:       "6a create with a name of 61 runes",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idC, strings.Repeat("é", 61)),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"name is longer than 60 characters"}`,
		},
		{
			name:       "6b create with a name of 60 runes",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idC, name60),
			wantStatus: http.StatusCreated,
			wantBody:   habitC,
		},
		{
			name:       "7 create with a taken id",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idA, "again"),
			wantStatus: http.StatusConflict,
			wantBody:   `{"error":"habit already exists"}`,
		},
		{
			name:       "8a create with an unknown field",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       `{"id":"` + idD + `","name":"x","extra":1}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidJSON,
		},
		{
			name:       "8b create with a truncated body",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidJSON,
		},
		{
			name:       "8c create with a body of 5000 bytes",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       oversized,
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidJSON,
		},
		{
			name:       "8d create with a second JSON value",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       `{"id":"` + idD + `"}{}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidJSON,
		},
		{
			name:       "9a rename",
			method:     http.MethodPatch,
			path:       "/api/habits/" + idA,
			body:       `{"name":"New"}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"` + idA + `","name":"New"}`,
		},
		{
			name:       "9b rename an unknown habit",
			method:     http.MethodPatch,
			path:       "/api/habits/" + idD,
			body:       `{"name":"x"}`,
			wantStatus: http.StatusNotFound,
			wantBody:   notFound,
		},
		{
			name:       "9c rename without a name",
			method:     http.MethodPatch,
			path:       "/api/habits/" + idA,
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"name is required"}`,
		},
		{
			name:       "9d rename with a malformed id",
			method:     http.MethodPatch,
			path:       "/api/habits/abc",
			body:       `{"name":"x"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidID,
		},
		{
			name:       "10a mark a day",
			method:     http.MethodPut,
			path:       "/api/habits/" + idA + "/days/2026-10-31",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "10b state lists the marked day",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   stateJSON("week", defaultAccent, habitJSON(idA, "New", "2026-10-31"), habitB, habitC),
		},
		{
			name:       "10c mark a day that is not in the calendar",
			method:     http.MethodPut,
			path:       "/api/habits/" + idA + "/days/2026-13-01",
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidDay,
		},
		{
			name:       "10d mark a day without zero padding",
			method:     http.MethodPut,
			path:       "/api/habits/" + idA + "/days/2026-1-1",
			wantStatus: http.StatusBadRequest,
			wantBody:   invalidDay,
		},
		{
			name:       "10e mark a day of an unknown habit",
			method:     http.MethodPut,
			path:       "/api/habits/" + idD + "/days/2026-10-31",
			wantStatus: http.StatusNotFound,
			wantBody:   notFound,
		},
		{
			name:       "10f unmark a day",
			method:     http.MethodDelete,
			path:       "/api/habits/" + idA + "/days/2026-10-31",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "11a delete",
			method:     http.MethodDelete,
			path:       "/api/habits/" + idB,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "11b delete again",
			method:     http.MethodDelete,
			path:       "/api/habits/" + idB,
			wantStatus: http.StatusNotFound,
			wantBody:   notFound,
		},
		{
			name:       "11c state without the deleted habit",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   stateJSON("week", defaultAccent, habitA, habitC),
		},
		{
			name:       "11d restore",
			method:     http.MethodPost,
			path:       "/api/habits/" + idB + "/restore",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "11e state has the restored habit at its original index",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   stateJSON("week", defaultAccent, habitA, habitB, habitC),
		},
		{
			name:       "11f restore an unknown habit",
			method:     http.MethodPost,
			path:       "/api/habits/" + idD + "/restore",
			wantStatus: http.StatusNotFound,
			wantBody:   notFound,
		},
		{
			name:       "12a set the view",
			method:     http.MethodPut,
			path:       "/api/settings",
			body:       `{"view":"month"}`,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "12b state has the view",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   stateJSON("month", defaultAccent, habitA, habitB, habitC),
		},
		{
			name:       "12c set an unknown view",
			method:     http.MethodPut,
			path:       "/api/settings",
			body:       `{"view":"decade"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid view: want week, month or year"}`,
		},
		{
			name:       "12d set an accent outside the palette",
			method:     http.MethodPut,
			path:       "/api/settings",
			body:       `{"accent":"#000000"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid accent: want one of the palette colors"}`,
		},
		{
			name:       "12e set nothing",
			method:     http.MethodPut,
			path:       "/api/settings",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"settings: nothing to update"}`,
		},
		{
			name:       "12f set the accent",
			method:     http.MethodPut,
			path:       "/api/settings",
			body:       `{"accent":"#5B8CEB"}`,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "12g state has the accent and keeps the view",
			method:     http.MethodGet,
			path:       "/api/state",
			wantStatus: http.StatusOK,
			wantBody:   stateJSON("month", "#5B8CEB", habitA, habitB, habitC),
		},
		{
			name:       "13 state accepts GET only",
			method:     http.MethodPost,
			path:       "/api/state",
			wantStatus: http.StatusMethodNotAllowed,
			statusOnly: true,
		},
		{
			name:       "14 health",
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
	}

	srv := newServer(t, Options{AllowedHosts: []string{allowedHost}})
	for _, tc := range steps {
		ok := t.Run(tc.name, func(t *testing.T) {
			rec := srv.do(request(t, tc.method, tc.path, tc.body))

			if tc.statusOnly {
				if rec.Code != tc.wantStatus {
					t.Errorf("%s %s status = %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
				}
				return
			}
			assertResponse(t, rec, tc.wantStatus, tc.wantBody)
		})
		if !ok {
			// Every later step relies on the state this one should have left, so their failures would only be noise.
			break
		}
	}
}

func TestHealthzDown(t *testing.T) {
	srv := newServer(t, Options{AllowedHosts: []string{allowedHost}})
	if err := srv.store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}

	rec := srv.do(request(t, http.MethodGet, "/healthz", ""))

	assertResponse(t, rec, http.StatusServiceUnavailable, `{"error":"database unavailable"}`)
}

func TestInternalError(t *testing.T) {
	var logs bytes.Buffer
	srv := newLoggedServer(t, Options{AllowedHosts: []string{allowedHost}}, zerolog.New(&logs))
	if err := srv.store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
	// The error the handler is about to hit, so the test can look for its text in the response and in the log.
	_, cause := srv.store.State(t.Context(), srv.ownerID)
	if cause == nil {
		t.Fatal("State on a closed store succeeded")
	}

	rec := srv.do(request(t, http.MethodGet, "/api/state", ""))

	assertResponse(t, rec, http.StatusInternalServerError, `{"error":"internal error"}`)
	if strings.Contains(rec.Body.String(), cause.Error()) {
		t.Errorf("body %q leaks the error text %q", rec.Body.String(), cause.Error())
	}
	// Decoding the whole output as one object also fails on a second log line.
	var entry struct {
		Level  string `json:"level"`
		Error  string `json:"error"`
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("log output %q is not exactly one JSON object: %v", logs.String(), err)
	}
	if entry.Level != "error" {
		t.Errorf("log level = %q, want %q", entry.Level, "error")
	}
	if entry.Error != "" || strings.Contains(logs.String(), cause.Error()) {
		t.Error("authentication log leaked infrastructure error")
	}
}

func TestStatic(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		wantStatus      int
		wantContentType string // with wantBody, checked on a 200 only
		wantBody        string
	}{
		{
			name:            "index page",
			path:            "/",
			wantStatus:      http.StatusOK,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        indexHTML,
		},
		{
			name:            "stylesheet",
			path:            "/styles.css",
			wantStatus:      http.StatusOK,
			wantContentType: "text/css; charset=utf-8",
			wantBody:        stylesCSS,
		},
		{
			name:            "script in dist",
			path:            "/dist/main.js",
			wantStatus:      http.StatusOK,
			wantContentType: "text/javascript; charset=utf-8",
			wantBody:        mainJS,
		},
		{
			name:       "script that dist does not have",
			path:       "/dist/missing.js",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "dist directory is not listed",
			path:       "/dist/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "file in dist that is not a script",
			path:       "/dist/notes.txt",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown path",
			path:       "/nope",
			wantStatus: http.StatusNotFound,
		},
	}
	srv := newServer(t, Options{AllowedHosts: []string{allowedHost}})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := srv.do(request(t, http.MethodGet, tc.path, ""))

			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s status = %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if got := rec.Header().Get("Content-Type"); got != tc.wantContentType {
				t.Errorf("GET %s Content-Type = %q, want %q", tc.path, got, tc.wantContentType)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("GET %s body = %q, want %q", tc.path, got, tc.wantBody)
			}
		})
	}
}
