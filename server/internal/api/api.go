package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"musicmirror/server/internal/catalog"
	"musicmirror/server/internal/rooms"
)

type Options struct {
	Origins    []string
	MaxUpload  int64
	TrustProxy bool
	Media      http.Handler
}

type API struct {
	rooms   *rooms.Manager
	songs   *catalog.Service
	opts    Options
	log     *slog.Logger
	limits  *limiter
	uploads chan struct{}
}

func New(r *rooms.Manager, s *catalog.Service, opts Options, log *slog.Logger) *API {
	return &API{rooms: r, songs: s, opts: opts, log: log, limits: &limiter{entries: map[string]window{}}, uploads: make(chan struct{}, 4)}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := a.songs.Ping(ctx); err != nil {
			publicError(w, 503, "storage is temporarily unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/rooms", a.create)
	mux.HandleFunc("POST /api/rooms/{code}/join", a.join)
	mux.HandleFunc("GET /api/session", a.session)
	mux.HandleFunc("DELETE /api/session", a.leave)
	mux.HandleFunc("GET /api/songs", a.list)
	mux.HandleFunc("POST /api/songs", a.upload)
	mux.HandleFunc("GET /api/ws", a.websocket)
	if a.opts.Media != nil {
		mux.Handle("/media/", a.opts.Media)
	}
	return a.middleware(mux)
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
func publicError(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}

func token(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}

func tokenKey(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
func (a *API) authorized(w http.ResponseWriter, r *http.Request) (rooms.Snapshot, bool) {
	state, err := a.rooms.Snapshot(token(r))
	if err != nil {
		publicError(w, 401, rooms.ErrSession.Error())
		return rooms.Snapshot{}, false
	}
	return state, true
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	data, err := io.ReadAll(r.Body)
	data = bytes.TrimSpace(data)
	if err != nil || len(data) == 0 || data[0] != '{' {
		publicError(w, 400, "request must be a JSON object")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		publicError(w, 400, "invalid JSON request")
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		publicError(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	if !a.limits.allow("join:"+a.ip(r), 20, time.Minute) {
		publicError(w, 429, "too many room requests; try again in a minute")
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	session, err := a.rooms.Create()
	if err != nil {
		publicError(w, 503, err.Error())
		return
	}
	respond(w, 201, session)
}
func (a *API) join(w http.ResponseWriter, r *http.Request) {
	if !a.limits.allow("join:"+a.ip(r), 20, time.Minute) {
		publicError(w, 429, "too many room requests; try again in a minute")
		return
	}
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	session, err := a.rooms.Join(strings.ToLower(r.PathValue("code")))
	if err != nil {
		status := 404
		if errors.Is(err, rooms.ErrCapacity) {
			status = 409
		}
		publicError(w, status, err.Error())
		return
	}
	respond(w, 201, session)
}
func (a *API) session(w http.ResponseWriter, r *http.Request) {
	state, ok := a.authorized(w, r)
	if ok {
		respond(w, 200, map[string]any{"room": state})
	}
}
func (a *API) leave(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authorized(w, r); !ok {
		return
	}
	a.rooms.Leave(token(r))
	w.WriteHeader(204)
}
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	state, ok := a.authorized(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	songs, err := a.songs.List(ctx, state.Code)
	if err != nil {
		a.log.Error("list songs failed", "error", err)
		publicError(w, 503, "could not load songs; please retry")
		return
	}
	respond(w, 200, map[string]any{"songs": songs})
}
func (a *API) upload(w http.ResponseWriter, r *http.Request) {
	state, ok := a.authorized(w, r)
	if !ok {
		return
	}
	if !a.limits.allow("upload:"+tokenKey(token(r)), 10, time.Minute) {
		publicError(w, 429, "too many uploads; try again in a minute")
		return
	}
	select {
	case a.uploads <- struct{}{}:
		defer func() { <-a.uploads }()
	default:
		publicError(w, 503, "uploads are busy; please retry")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.opts.MaxUpload+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		publicError(w, 413, "upload is too large or invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["song"]) != 1 || len(r.MultipartForm.Value) != 0 {
		publicError(w, 400, "provide exactly one file in the song field")
		return
	}
	file, header, err := r.FormFile("song")
	if err != nil {
		publicError(w, 400, "choose an audio file")
		return
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	song, err := a.songs.Upload(ctx, state.Code, header.Filename, file)
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrUpload):
			publicError(w, 400, err.Error())
		case errors.Is(err, catalog.ErrQuota):
			publicError(w, 409, err.Error())
		default:
			a.log.Error("upload failed", "error", err)
			publicError(w, 503, "could not save audio; please retry")
		}
		return
	}
	a.rooms.NotifyPlaylist(token(r))
	respond(w, 201, map[string]any{"song": song})
}

func (a *API) ip(r *http.Request) string {
	if a.opts.TrustProxy {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		ip := strings.TrimSpace(parts[len(parts)-1])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				a.log.Error("request panic", "panic", fmt.Sprint(value))
				publicError(w, 500, "internal server error")
			}
		}()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, o := range a.opts.Origins {
				if o == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				publicError(w, 403, "origin is not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !a.limits.allow("http:"+a.ip(r), 300, time.Minute) {
			publicError(w, 429, "too many requests; please wait a minute")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type window struct {
	start time.Time
	count int
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]window
}

func (l *limiter) allow(key string, max int, duration time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) >= 4096 {
		for k, v := range l.entries {
			if now.Sub(v.start) > time.Minute {
				delete(l.entries, k)
			}
		}
	}
	v, ok := l.entries[key]
	if !ok && len(l.entries) >= 4096 {
		return false
	}
	if now.Sub(v.start) >= duration {
		v = window{start: now}
	}
	v.count++
	l.entries[key] = v
	return v.count <= max
}
