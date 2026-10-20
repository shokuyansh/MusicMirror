package catalog

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TEST_DATABASE_URL must name a dedicated test database. CI provides one.
func TestPostgresRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to exercise real PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	if err = p.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id := "integration-" + time.Now().Format("150405.000000000")
	now := time.Now().UTC().Truncate(time.Microsecond)
	expires := now.Add(time.Minute)
	song := Song{ID: id, Room: id, Name: "round trip.wav", ObjectKey: id, ContentType: "audio/wav", Size: 128, CreatedAt: now, ExpiresAt: &expires}
	t.Cleanup(func() { p.Delete(context.Background(), id) })
	if err = p.Create(ctx, song); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	before, err := p.List(ctx, id, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range before {
		if s.ID == id {
			t.Fatal("pending object visible")
		}
	}
	if err = p.MarkReady(ctx, id); err != nil {
		t.Fatal(err)
	}
	read, err := p.Get(ctx, id)
	if err != nil || !read.Ready || read.Name != song.Name || read.ExpiresAt == nil {
		t.Fatalf("%+v %v", read, err)
	}
	list, err := p.List(ctx, id, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range list {
		if s.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("ready song missing")
	}
	if total, err := p.TotalSize(ctx); err != nil || total < 128 {
		t.Fatalf("%v %v", total, err)
	}
	if err = p.ExpireRooms(ctx, []string{id}, now); err != nil {
		t.Fatal(err)
	}
	expired, err := p.Expired(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, s := range expired {
		if s.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("expired song missing")
	}
	if err = p.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
