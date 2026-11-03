package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/usecase"
)

type identityKey struct{}

func requestUser(r *http.Request) string {
	user, _ := r.Context().Value(identityKey{}).(string)
	return user
}
func sessionBinding(token string) string {
	sum := sha256.Sum256([]byte("steady-request:" + token))
	return hex.EncodeToString(sum[:])
}
func requestToken(r *http.Request) string {
	c, err := r.Cookie("steady_session")
	if err != nil {
		return ""
	}
	return c.Value
}
func (h *handler) current(w http.ResponseWriter, r *http.Request) (domain.Account, bool) {
	account, err := h.accounts.Current(r.Context(), requestToken(r))
	if err != nil {
		h.authError(w, err)
		return domain.Account{}, false
	}
	return account, true
}
func (h *handler) private(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account, ok := h.current(w, r)
		if !ok {
			return
		}
		expected := sessionBinding(requestToken(r))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(r.Header.Get("X-Steady-Session"))) != 1 {
			writeError(w, 401, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, account.ID)))
	}
}
func (h *handler) authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		writeError(w, 401, "authentication required")
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, 401, "invalid credentials")
	default:
		h.log.Error().Msg("authentication failed")
		writeError(w, 500, "internal error")
	}
}
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.allow(r.RemoteAddr) {
		writeError(w, 429, "too many login attempts")
		return
	}
	req, err := decodeBody[struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}](w, r)
	if err != nil {
		writeError(w, 400, "invalid JSON body")
		return
	}
	if !h.limiter.acquire() {
		writeError(w, 429, "too many login attempts")
		return
	}
	defer h.limiter.release()
	token, expiry, err := h.accounts.Login(r.Context(), req.Username, []byte(req.Password))
	if err != nil {
		h.authError(w, err)
		return
	}
	h.cookie(w, token, expiry, int(usecase.SessionLifetime/time.Second))
	w.WriteHeader(http.StatusNoContent)
}
func (h *handler) session(w http.ResponseWriter, r *http.Request) {
	account, ok := h.current(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, struct {
		Username string `json:"username"`
		Binding  string `json:"binding"`
	}{account.Username, sessionBinding(requestToken(r))})
}
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	token := requestToken(r)
	if token != "" {
		_, err := h.accounts.Current(r.Context(), token)
		if err == nil {
			if subtle.ConstantTimeCompare([]byte(sessionBinding(token)), []byte(r.Header.Get("X-Steady-Session"))) != 1 {
				writeError(w, 401, "authentication required")
				return
			}
		} else if !errors.Is(err, domain.ErrUnauthenticated) {
			h.authError(w, err)
			return
		}
	}
	if err := h.accounts.Logout(r.Context(), token); err != nil {
		h.authError(w, err)
		return
	}
	h.cookie(w, "", time.Unix(1, 0), -1)
	w.WriteHeader(http.StatusNoContent)
}
func (h *handler) cookie(w http.ResponseWriter, token string, expiry time.Time, maxAge int) {
	// CookieSecure is explicit operator configuration; local HTTP requires false.
	//nolint:gosec // HTTPS deployments default to Secure; the local HTTP exception is intentional.
	http.SetCookie(w, &http.Cookie{Name: "steady_session", Value: token, Path: "/", Secure: h.cookieSecure, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expiry, MaxAge: maxAge})
}

type loginWindow struct {
	started  time.Time
	attempts int
}
type loginLimiter struct {
	mu     sync.Mutex
	now    func() time.Time
	ips    map[string]loginWindow
	global loginWindow
	active int
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{now: now, ips: make(map[string]loginWindow)}
}
func (l *loginLimiter) allow(remoteAddr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	for key, window := range l.ips {
		if !now.Before(window.started.Add(15 * time.Minute)) {
			delete(l.ips, key)
		}
	}
	if l.global.started.IsZero() || !now.Before(l.global.started.Add(time.Minute)) {
		l.global = loginWindow{started: now}
	}
	entry, exists := l.ips[ip]
	if !exists {
		if len(l.ips) >= 1024 {
			return false
		}
		entry = loginWindow{started: now}
	}
	if l.global.attempts >= 30 || entry.attempts >= 5 {
		return false
	}
	entry.attempts++
	l.ips[ip] = entry
	l.global.attempts++
	return true
}
func (l *loginLimiter) acquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active >= 2 {
		return false
	}
	l.active++
	return true
}
func (l *loginLimiter) release() { l.mu.Lock(); defer l.mu.Unlock(); l.active-- }
