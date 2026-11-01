package httpapi

import (
	"net/http"
	"slices"
	"testing"
)

func TestHostAllowlist(t *testing.T) {
	const hostDenied = `{"error":"host not allowed"}`
	tests := []struct {
		name       string
		allowed    []string // Options.AllowedHosts; nil means []string{allowedHost}
		host       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "unknown host",
			host:       "evil.example",
			wantStatus: http.StatusForbidden,
			wantBody:   hostDenied,
		},
		{
			name:       "loopback address as the first labels of another name",
			host:       "127.0.0.1.evil.example",
			wantStatus: http.StatusForbidden,
			wantBody:   hostDenied,
		},
		{
			name:       "empty host",
			host:       "",
			wantStatus: http.StatusForbidden,
			wantBody:   hostDenied,
		},
		{
			name:       "localhost on any port",
			host:       "localhost:9999",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "IPv6 loopback in brackets",
			host:       "[::1]:8770",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "IPv4 loopback without a port",
			host:       "127.0.0.1",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "configured name",
			host:       "steady.example.com",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "configured name in another case",
			host:       "Steady.Example.COM",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "entry without a port matches any port",
			host:       "steady.example.com:8443",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "entry with a port matches that port",
			allowed:    []string{"proxy.example:8443"},
			host:       "proxy.example:8443",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
		},
		{
			name:       "entry with a port rejects another port",
			allowed:    []string{"proxy.example:8443"},
			host:       "proxy.example:9000",
			wantStatus: http.StatusForbidden,
			wantBody:   hostDenied,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			allowed := tc.allowed
			if allowed == nil {
				allowed = []string{allowedHost}
			}
			srv := newServer(t, Options{AllowedHosts: allowed})
			// Set even when empty: the request helper's default Host is not what this test is about.
			req := request(t, http.MethodGet, "/api/state", "")
			req.Host = tc.host

			assertResponse(t, srv.do(req), tc.wantStatus, tc.wantBody)
		})
	}
}

func TestCrossOrigin(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		fetchSite  string // Sec-Fetch-Site; empty sends none
		origin     string // Origin; empty sends none
		wantStatus int
		wantBody   string
		wantState  string // body of a GET /api/state sent afterwards
	}{
		{
			name:       "cross-site POST is rejected and stores nothing",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idA, "Read"),
			fetchSite:  "cross-site",
			wantStatus: http.StatusForbidden,
			wantBody:   `{"error":"cross-origin request rejected"}`,
			wantState:  emptyState,
		},
		{
			name:       "foreign Origin without Sec-Fetch-Site is rejected and stores nothing",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idA, "Read"),
			origin:     "http://evil.example",
			wantStatus: http.StatusForbidden,
			wantBody:   `{"error":"cross-origin request rejected"}`,
			wantState:  emptyState,
		},
		{
			name:       "same-origin POST goes through",
			method:     http.MethodPost,
			path:       "/api/habits",
			body:       createBody(idA, "Read"),
			fetchSite:  "same-origin",
			wantStatus: http.StatusCreated,
			wantBody:   habitJSON(idA, "Read"),
			wantState:  stateJSON("week", defaultAccent, habitJSON(idA, "Read")),
		},
		{
			name:       "cross-site GET is a safe method and goes through",
			method:     http.MethodGet,
			path:       "/api/state",
			fetchSite:  "cross-site",
			wantStatus: http.StatusOK,
			wantBody:   emptyState,
			wantState:  emptyState,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, Options{AllowedHosts: []string{allowedHost}})
			req := request(t, tc.method, tc.path, tc.body)
			// The helper marks a mutating request same-origin; this table decides the headers itself.
			req.Header.Del("Sec-Fetch-Site")
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			assertResponse(t, srv.do(req), tc.wantStatus, tc.wantBody)

			assertResponse(t, srv.do(request(t, http.MethodGet, "/api/state", "")), http.StatusOK, tc.wantState)
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	wantHeaders := []struct{ name, value string }{
		{
			name: "Content-Security-Policy",
			value: "default-src 'none'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; " +
				"font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; base-uri 'none'; " +
				"form-action 'none'; frame-ancestors 'none'",
		},
		{name: "X-Content-Type-Options", value: "nosniff"},
		{name: "Referrer-Policy", value: "no-referrer"},
		{name: "Cache-Control", value: "no-store"},
	}
	tests := []struct {
		name       string
		host       string
		path       string
		wantStatus int
	}{
		{name: "API response", host: loopbackHost, path: "/api/state", wantStatus: http.StatusOK},
		{name: "rejected Host", host: "evil.example", path: "/api/state", wantStatus: http.StatusForbidden},
		{name: "static file", host: loopbackHost, path: "/", wantStatus: http.StatusOK},
	}
	srv := newServer(t, Options{AllowedHosts: []string{allowedHost}})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := request(t, http.MethodGet, tc.path, "")
			req.Host = tc.host

			rec := srv.do(req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("GET %s with Host %q status = %d, want %d", tc.path, tc.host, rec.Code, tc.wantStatus)
			}
			for _, want := range wantHeaders {
				// Values, not Get: the header must be present exactly once.
				if got := rec.Header().Values(want.name); !slices.Equal(got, []string{want.value}) {
					t.Errorf("%s = %q, want %q", want.name, got, []string{want.value})
				}
			}
		})
	}
}
