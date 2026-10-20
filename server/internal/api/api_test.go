package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"musicmirror/server/internal/catalog"
	"musicmirror/server/internal/objects"
	"musicmirror/server/internal/rooms"
)

func testAPI(t *testing.T) (*API, *httptest.Server) {
	t.Helper()
	local, err := objects.NewLocal(t.TempDir(), bytes.Repeat([]byte("s"), 32), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	songs := catalog.New(catalog.NewMemory(), local, time.Now, 1024, 8192)
	a := New(rooms.New(time.Now), songs, Options{Origins: []string{"http://localhost:5173"}, MaxUpload: 1024, Media: local}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := httptest.NewServer(a.Handler())
	t.Cleanup(s.Close)
	return a, s
}
func request(t *testing.T, handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func createSession(t *testing.T, a *API) rooms.Session {
	t.Helper()
	w := request(t, a.Handler(), "POST", "/api/rooms", "", "{}")
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var s rooms.Session
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHTTPRoomAuthorizationCORSAndValidation(t *testing.T) {
	a, _ := testAPI(t)
	handler := a.Handler()
	for _, path := range []string{"/healthz", "/readyz"} {
		if w := request(t, handler, "GET", path, "", ""); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/session"}, {"GET", "/api/songs"}, {"POST", "/api/songs"}, {"DELETE", "/api/session"}} {
		if w := request(t, handler, route.method, route.path, "bad", ""); w.Code != 401 {
			t.Fatal(route, w.Code)
		}
	}
	for _, body := range []string{`{"room":"client-chosen"}`, `{} {}`, `null`, strings.Repeat("x", 2048)} {
		w := request(t, handler, "POST", "/api/rooms", "", body)
		if w.Code != 400 {
			t.Fatal(body, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/rooms", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("OPTIONS", "/api/rooms", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal(w)
	}
	session := createSession(t, a)
	w = request(t, handler, "POST", "/api/rooms/"+strings.ToUpper(session.Room.Code)+"/join", "", "{}")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, handler, "POST", "/api/rooms/ffffff0/join", "", "{}")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = request(t, handler, "GET", "/api/session", session.Token, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request(t, handler, "DELETE", "/api/session", session.Token, "")
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	w = request(t, handler, "GET", "/api/session", session.Token, "")
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestUploadAndPlaylistIsolation(t *testing.T) {
	a, _ := testAPI(t)
	owner, other := createSession(t, a), createSession(t, a)
	data := make([]byte, 128)
	copy(data, "RIFF")
	copy(data[8:], "WAVEfmt ")
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, _ := form.CreateFormFile("song", "demo.wav")
	file.Write(data)
	form.Close()
	r := httptest.NewRequest("POST", "/api/songs", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+owner.Token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = request(t, a.Handler(), "GET", "/api/songs", owner.Token, "")
	var list struct {
		Songs []catalog.Song `json:"songs"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Songs) != 1 || list.Songs[0].URL == "" {
		t.Fatal(w.Body)
	}
	w = request(t, a.Handler(), "GET", "/api/songs", other.Token, "")
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Songs) != 0 {
		t.Fatal("cross-room playlist leak")
	}
	response := a.command(context.Background(), other.Token, message{Type: "select", SongID: "missing", Revision: 0})
	if response.Type != "error" {
		t.Fatal(response)
	}
}

func dial(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/api/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if err = wsjson.Write(ctx, c, message{Type: "auth", Token: token}); err != nil {
		t.Fatal(err)
	}
	return c
}
func readEvent(t *testing.T, c *websocket.Conn, match func(event) bool) event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		var e event
		if err := wsjson.Read(ctx, c, &e); err != nil {
			t.Fatal(err)
		}
		if match(e) {
			return e
		}
	}
}
func writeMessage(t *testing.T, c *websocket.Conn, m message) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, c, m); err != nil {
		t.Fatal(err)
	}
}

func TestWebSocketMultiplayerSeekLateJoinAndLocalPause(t *testing.T) {
	a, server := testAPI(t)
	owner := createSession(t, a)
	data := make([]byte, 128)
	copy(data, "RIFF")
	copy(data[8:], "WAVEfmt ")
	song, err := a.songs.Upload(context.Background(), owner.Room.Code, "demo.wav", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	one := dial(t, server.URL, owner.Token)
	readEvent(t, one, func(e event) bool { return e.Type == "snapshot" })
	writeMessage(t, one, message{Type: "select", SongID: song.ID, Revision: 0})
	readEvent(t, one, func(e event) bool { return e.Room != nil && e.Room.Playback.Revision == 1 })
	guest, err := a.rooms.Join(owner.Room.Code)
	if err != nil {
		t.Fatal(err)
	}
	two := dial(t, server.URL, guest.Token)
	late := readEvent(t, two, func(e event) bool { return e.Type == "snapshot" })
	if late.Room.Playback.SongID != song.ID {
		t.Fatal("late join lost current song")
	}
	writeMessage(t, two, message{Type: "seek", Position: 32, Revision: 1})
	for _, c := range []*websocket.Conn{one, two} {
		e := readEvent(t, c, func(e event) bool { return e.Room != nil && e.Room.Playback.Revision == 2 })
		if e.Room.Playback.Position < 32 || e.Room.Playback.Position > 33 {
			t.Fatal(e.Room.Playback)
		}
	}
	writeMessage(t, one, message{Type: "seek", Position: 1, Revision: 1})
	stale := readEvent(t, one, func(e event) bool { return e.Error != "" })
	if stale.Room == nil || stale.Room.Playback.Revision != 2 {
		t.Fatal(stale)
	}
	writeMessage(t, two, message{Type: "pause"})
	readEvent(t, two, func(e event) bool { return e.Type == "error" })
	writeMessage(t, two, message{Type: "ping", ClientTime: 123})
	pong := readEvent(t, two, func(e event) bool { return e.Type == "pong" })
	if pong.ClientTime != 123 || pong.ServerTime == 0 {
		t.Fatal(pong)
	}
	state, _ := a.rooms.Snapshot(owner.Token)
	if state.Playback.Revision != 2 {
		t.Fatal("local pause mutated room")
	}
	// Re-authentication on another socket replaces transport, preserving identity.
	reconnected := dial(t, server.URL, guest.Token)
	stateEvent := readEvent(t, reconnected, func(e event) bool { return e.Type == "snapshot" })
	if len(stateEvent.Room.Members) != 2 {
		t.Fatal("reconnect duplicated member")
	}
}

func TestWebSocketRejectsUnauthenticatedAndForeignOrigin(t *testing.T) {
	_, server := testAPI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	wsjson.Write(ctx, c, message{Type: "auth", Token: "bad"})
	var e event
	err = wsjson.Read(ctx, c, &e)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatal(err)
	}
	_, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}}})
	if err == nil || res.StatusCode != 403 {
		t.Fatal("foreign websocket origin accepted", err)
	}
}
