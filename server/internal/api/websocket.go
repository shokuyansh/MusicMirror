package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"musicmirror/server/internal/catalog"
	"musicmirror/server/internal/rooms"
)

type message struct {
	Type       string  `json:"type"`
	Token      string  `json:"token,omitempty"`
	SongID     string  `json:"songId,omitempty"`
	Position   float64 `json:"position,omitempty"`
	Revision   uint64  `json:"revision,omitempty"`
	ClientTime float64 `json:"clientTime,omitempty"`
}
type event struct {
	Type       string          `json:"type"`
	Room       *rooms.Snapshot `json:"room,omitempty"`
	Error      string          `json:"error,omitempty"`
	ClientTime float64         `json:"clientTime,omitempty"`
	ServerTime int64           `json:"serverTime,omitempty"`
}

func (a *API) websocket(w http.ResponseWriter, r *http.Request) {
	patterns := []string{}
	for _, origin := range a.opts.Origins {
		u, _ := url.Parse(origin)
		patterns = append(patterns, u.Host)
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: patterns})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
	var auth message
	err = wsjson.Read(authCtx, c, &auth)
	authCancel()
	if err != nil || auth.Type != "auth" || auth.Token == "" {
		c.Close(websocket.StatusPolicyViolation, "authenticate first")
		return
	}
	updates, detach, err := a.rooms.Subscribe(auth.Token)
	if err != nil {
		c.Close(websocket.StatusPolicyViolation, "session expired")
		return
	}
	defer detach()
	outgoing := make(chan event, 32)
	// One read loop and one write loop per peer. No unbounded queues or goroutines.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		for {
			var command message
			if err := wsjson.Read(ctx, c, &command); err != nil {
				return
			}
			if !a.limits.allow("ws:"+tokenKey(auth.Token), 180, time.Minute) {
				c.Close(websocket.StatusPolicyViolation, "too many messages")
				return
			}
			response := a.command(ctx, auth.Token, command)
			select {
			case outgoing <- response:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); c.CloseNow(); <-done }()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	write := func(e event) error {
		writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		return wsjson.Write(writeCtx, c, e)
	}
	snapshot := func() event {
		room, err := a.rooms.Snapshot(auth.Token)
		if err != nil {
			return event{Type: "expired", Error: rooms.ErrSession.Error()}
		}
		return event{Type: "snapshot", Room: &room}
	}
	if err = write(snapshot()); err != nil {
		return
	}
	for {
		var e event
		select {
		case <-ctx.Done():
			return
		case response := <-outgoing:
			e = response
		case kind, open := <-updates:
			if !open {
				c.Close(websocket.StatusPolicyViolation, "connection replaced or room expired")
				return
			}
			if kind == "playlist" {
				e = event{Type: "playlist"}
			} else {
				e = snapshot()
			}
		case <-ticker.C:
			e = snapshot()
		}
		if err = write(e); err != nil {
			return
		}
		if e.Type == "expired" {
			return
		}
	}
}

func (a *API) command(ctx context.Context, token string, cmd message) event {
	state, err := a.rooms.Snapshot(token)
	if err != nil {
		return event{Type: "expired", Error: rooms.ErrSession.Error()}
	}
	switch cmd.Type {
	case "ping":
		return event{Type: "pong", ClientTime: cmd.ClientTime, ServerTime: time.Now().UnixMilli()}
	case "state":
		return event{Type: "snapshot", Room: &state}
	case "select":
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err = a.songs.Get(lookupCtx, state.Code, cmd.SongID); err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				return event{Type: "error", Error: catalog.ErrNotFound.Error()}
			}
			a.log.Error("look up song failed", "error", err)
			return event{Type: "error", Error: "could not select song; please retry"}
		}
		state, err = a.rooms.Select(token, cmd.SongID, cmd.Revision)
	case "seek":
		state, err = a.rooms.Seek(token, cmd.Position, cmd.Revision)
	default:
		return event{Type: "error", Error: "unsupported playback command"}
	}
	if errors.Is(err, rooms.ErrConflict) {
		return event{Type: "snapshot", Room: &state, Error: err.Error()}
	}
	if err != nil {
		return event{Type: "error", Error: err.Error()}
	}
	return event{Type: "snapshot", Room: &state}
}
