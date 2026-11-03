// Package httpapi is the HTTP adapter of Steady: the JSON API in front of the use case service, the embedded
// frontend files, and the middleware that guards both.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"time"

	"github.com/rs/zerolog"

	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/usecase"
)

// maxBodyBytes caps a request body; a valid one is a UUID and a name of at most 60 runes.
const maxBodyBytes = 4096

// errInvalidBody is the client error for a body that is not exactly one JSON value of the expected shape.
const errInvalidBody domain.ValidationError = "invalid JSON body"

// Content types of the static files.
const (
	typeHTML = "text/html; charset=utf-8"
	typeCSS  = "text/css; charset=utf-8"
	typeJS   = "text/javascript; charset=utf-8"
)

// Options configures New.
type Options struct {
	// AllowedHosts are the Host header values accepted besides the loopback names. An entry with a port must
	// equal the whole header; an entry without one matches that hostname on any port.
	Accounts     *usecase.Accounts
	CookieSecure bool
	AllowedHosts []string
}

// habitDTO is a habit on the wire; Done is always an array.
type habitDTO struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Done []string `json:"done"`
}

// stateDTO is the body of GET /api/state; Habits is always an array.
type stateDTO struct {
	Habits []habitDTO `json:"habits"`
	View   string     `json:"view"`
	Accent string     `json:"accent"`
}

// nameDTO is the body of a successful rename.
type nameDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// errorDTO is the body of every error response.
type errorDTO struct {
	Error string `json:"error"`
}

// statusDTO is the body of a healthy GET /healthz.
type statusDTO struct {
	Status string `json:"status"`
}

type createReq struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type renameReq struct {
	Name *string `json:"name"`
}

type settingsReq struct {
	View   *string `json:"view"`
	Accent *string `json:"accent"`
}

// asset is a static file kept in memory, so only files read at startup can ever be served.
type asset struct {
	body        []byte
	contentType string
}

func (a asset) serve(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", a.contentType)
	// The status line is already out, so a failed write (the client went away) has nowhere to be reported.
	_, _ = w.Write(a.body)
}

// handler holds what the route handlers share.
type handler struct {
	accounts     *usecase.Accounts
	cookieSecure bool
	limiter      *loginLimiter
	svc          *usecase.Service
	log          zerolog.Logger
	scripts      map[string]asset // dist/*.js by file name
}

// New returns the HTTP handler of Steady: the JSON API over svc and the files of static, behind the security
// headers, the Host allowlist and the cross-origin check. static holds index.html, styles.css and dist/*.js at its
// root; New reads them into memory, so nothing else in static is ever served.
func New(svc *usecase.Service, static fs.FS, opts Options, log zerolog.Logger) (http.Handler, error) {
	if opts.Accounts == nil {
		return nil, fmt.Errorf("httpapi: accounts required")
	}
	index, err := readAsset(static, "index.html", typeHTML)
	if err != nil {
		return nil, err
	}
	styles, err := readAsset(static, "styles.css", typeCSS)
	if err != nil {
		return nil, err
	}
	names, err := fs.Glob(static, "dist/*.js")
	if err != nil {
		return nil, fmt.Errorf("httpapi: glob dist/*.js: %w", err)
	}
	scripts := make(map[string]asset, len(names))
	for _, name := range names {
		script, err := readAsset(static, name, typeJS)
		if err != nil {
			return nil, err
		}
		scripts[path.Base(name)] = script
	}

	h := &handler{svc: svc, log: log, scripts: scripts, accounts: opts.Accounts, cookieSecure: opts.CookieSecure, limiter: newLoginLimiter(time.Now)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", index.serve)
	mux.HandleFunc("GET /styles.css", styles.serve)
	mux.HandleFunc("GET /dist/{file}", h.script)
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("GET /api/session", h.session)
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/state", h.private(h.state))
	mux.HandleFunc("POST /api/habits", h.private(h.createHabit))
	mux.HandleFunc("PATCH /api/habits/{id}", h.private(h.renameHabit))
	mux.HandleFunc("DELETE /api/habits/{id}", h.private(h.deleteHabit))
	mux.HandleFunc("POST /api/habits/{id}/restore", h.private(h.restoreHabit))
	mux.HandleFunc("PUT /api/habits/{id}/days/{day}", h.private(h.setDay(true)))
	mux.HandleFunc("DELETE /api/habits/{id}/days/{day}", h.private(h.setDay(false)))
	mux.HandleFunc("PUT /api/settings", h.private(h.updateSettings))
	mux.HandleFunc("GET /healthz", h.healthz)
	return secureHeaders(allowHosts(opts.AllowedHosts, crossOrigin(mux))), nil
}

// readAsset reads one static file into memory.
func readAsset(static fs.FS, name, contentType string) (asset, error) {
	body, err := fs.ReadFile(static, name)
	if err != nil {
		return asset{}, fmt.Errorf("httpapi: read %s: %w", name, err)
	}
	return asset{body: body, contentType: contentType}, nil
}

func (h *handler) script(w http.ResponseWriter, r *http.Request) {
	script, ok := h.scripts[r.PathValue("file")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	script.serve(w, r)
}

func (h *handler) state(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.State(r.Context(), requestUser(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newStateDTO(st))
}

func (h *handler) createHabit(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[createReq](w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	habit, err := h.svc.AddHabit(r.Context(), requestUser(r), req.ID, req.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newHabitDTO(habit))
}

func (h *handler) renameHabit(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[renameReq](w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	habit, err := h.svc.RenameHabit(r.Context(), requestUser(r), r.PathValue("id"), req.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nameDTO{ID: habit.ID, Name: habit.Name})
}

func (h *handler) deleteHabit(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteHabit(r.Context(), requestUser(r), r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) restoreHabit(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RestoreHabit(r.Context(), requestUser(r), r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setDay returns the handler that marks (done) or unmarks a day; both are idempotent.
func (h *handler) setDay(done bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.svc.SetDay(r.Context(), requestUser(r), r.PathValue("id"), r.PathValue("day"), done); err != nil {
			h.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[settingsReq](w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.UpdateSettings(r.Context(), requestUser(r), req.View, req.Accent); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Ping(r.Context()); err != nil {
		h.log.Warn().Err(err).Msg("health check failed")
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, statusDTO{Status: "ok"})
}

// fail answers a use case or decode error: bad input is 400 with the text of its domain error, a missing habit
// 404, a taken id 409, and anything else 500 with a fixed text, so internals never reach the client.
func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var invalid domain.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, domain.ErrNotFound.Error())
	case errors.Is(err, domain.ErrExists):
		writeError(w, http.StatusConflict, domain.ErrExists.Error())
	default:
		h.log.Error().Err(err).Str("method", r.Method).Str("path", r.URL.Path).Msg("request failed")
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// decodeBody reads the request body as exactly one JSON value into a T. Unknown fields, trailing data, a body
// over maxBodyBytes and malformed JSON all give errInvalidBody.
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v, zero T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return zero, errInvalidBody
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return zero, errInvalidBody
	}
	return v, nil
}

func newHabitDTO(h domain.Habit) habitDTO {
	done := h.Done
	if done == nil {
		done = []string{}
	}
	return habitDTO{ID: h.ID, Name: h.Name, Done: done}
}

func newStateDTO(st domain.State) stateDTO {
	habits := make([]habitDTO, 0, len(st.Habits))
	for _, h := range st.Habits {
		habits = append(habits, newHabitDTO(h))
	}
	return stateDTO{Habits: habits, View: string(st.View), Accent: st.Accent}
}

// writeJSON sends v as compact JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status line is already out, so a failed write (the client went away) has nowhere to be reported.
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": msg} with the given status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorDTO{Error: msg})
}
