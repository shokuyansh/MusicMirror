package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"musicmirror/server/internal/bootstrap"
	"musicmirror/server/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	runtime, err := bootstrap.Open(startup, c, log)
	cancel()
	if err != nil {
		return err
	}
	defer runtime.Close()
	sweepDone := make(chan struct{})
	go func() { defer close(sweepDone); runtime.Sweep(ctx, log) }()
	defer func() { stop(); <-sweepDone }()
	server := &http.Server{Addr: ":" + c.Port, Handler: runtime.Handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 75 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	errCh := make(chan error, 1)
	go func() { log.Info("listening", "port", c.Port); errCh <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err = <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
