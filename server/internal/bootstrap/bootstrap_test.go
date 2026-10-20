package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"musicmirror/server/internal/config"
)

func TestLocalStartupReadinessAndSweepShutdown(t *testing.T) {
	c, err := config.From(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	c.DataDir = t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r, err := Open(context.Background(), c, log)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w := httptest.NewRecorder()
	r.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Sweep(ctx, log); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not stop")
	}
}
