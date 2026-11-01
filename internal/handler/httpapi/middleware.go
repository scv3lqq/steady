package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// contentSecurityPolicy lets the page load only its own script and style plus Google Fonts, and talk only to itself.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; " +
	"font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; base-uri 'none'; " +
	"form-action 'none'; frame-ancestors 'none'"

// secureHeaders sets the security headers before anything else runs, so every response carries them, the
// rejections of the inner middleware included.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// allowHosts answers 403 to a request whose Host is not allowed by hostAllowed.
func allowHosts(extra []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, extra) {
			writeError(w, http.StatusForbidden, "host not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether a Host header value may reach the server. Checking Host defeats DNS rebinding: a page
// that points its own name at the loopback address still sends that name, which is not on the list. The loopback
// names pass on any port, so an SSH tunnel on another local port works. An entry of extra with a port must equal the
// whole Host; an entry without one matches the hostname on any port. Case is ignored.
func hostAllowed(host string, extra []string) bool {
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	name, _ := splitHost(host)
	switch name {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	for _, entry := range extra {
		entry = strings.ToLower(entry)
		entryName, entryHasPort := splitHost(entry)
		switch {
		case entryHasPort && entry == host:
			return true
		case !entryHasPort && entryName == name:
			return true
		}
	}
	return false
}

// splitHost returns the hostname of a Host header value or an allowlist entry, without IPv6 brackets, and whether the
// value carries a port.
func splitHost(v string) (string, bool) {
	if host, _, err := net.SplitHostPort(v); err == nil {
		return host, true
	}
	return strings.Trim(v, "[]"), false
}

// crossOrigin answers 403 to a cross-origin browser request that uses a non-safe method. Those are exactly the
// mutating routes, because the GET routes never change state.
func crossOrigin(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
	}))
	return cop.Handler(next)
}
