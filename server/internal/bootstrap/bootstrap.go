package bootstrap

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"musicmirror/server/internal/api"
	"musicmirror/server/internal/catalog"
	"musicmirror/server/internal/config"
	"musicmirror/server/internal/objects"
	"musicmirror/server/internal/rooms"
)

type Runtime struct {
	Handler http.Handler
	Catalog *catalog.Service
	Rooms   *rooms.Manager
	Close   func()
}

func Open(ctx context.Context, c config.Config, log *slog.Logger) (*Runtime, error) {
	var repo catalog.Repository
	var blobs catalog.Objects
	var media http.Handler
	closeFn := func() {}
	if c.DatabaseURL != "" {
		pg, err := catalog.OpenPostgres(ctx, c.DatabaseURL)
		if err != nil {
			return nil, err
		}
		if err = pg.Migrate(ctx); err != nil {
			pg.Close()
			return nil, err
		}
		repo = pg
		closeFn = func() { pg.Close() }
		blobs = objects.NewSupabase(c.SupabaseURL, c.SupabaseKey, c.Bucket, &http.Client{Timeout: 45 * time.Second})
	} else {
		secret := make([]byte, 32)
		rand.Read(secret)
		local, err := objects.NewLocal(c.DataDir, secret, time.Now)
		if err != nil {
			return nil, err
		}
		repo = catalog.NewMemory()
		blobs = local
		media = local
		log.Info("local development storage enabled; metadata resets on restart")
	}
	manager := rooms.New(time.Now)
	songs := catalog.New(repo, blobs, time.Now, c.MaxUpload, c.MaxStorage)
	handler := api.New(manager, songs, api.Options{Origins: c.Origins, MaxUpload: c.MaxUpload, TrustProxy: c.TrustProxy, Media: media}, log).Handler()
	return &Runtime{Handler: handler, Catalog: songs, Rooms: manager, Close: closeFn}, nil
}

// Sweep remembers expired room IDs until persistence succeeds. The metadata
// expiry index also recovers pending cleanup after process restarts.
func (r *Runtime) Sweep(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var pending []string
	for {
		pending = append(pending, r.Rooms.Expire()...)
		jobCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		err := r.Catalog.Cleanup(jobCtx, pending)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Error("audio cleanup will retry", "error", err)
		} else if err == nil {
			pending = nil
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
