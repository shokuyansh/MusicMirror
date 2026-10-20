package catalog

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeObjects struct {
	objects             map[string][]byte
	failPut, failDelete bool
}

func (f *fakeObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	if f.failPut {
		return errors.New("upload failed")
	}
	b, err := io.ReadAll(r)
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[key] = b
	return err
}
func (f *fakeObjects) Delete(_ context.Context, key string) error {
	if f.failDelete {
		return errors.New("delete failed")
	}
	delete(f.objects, key)
	return nil
}
func (f *fakeObjects) URL(_ context.Context, key string) (string, error) {
	return "https://audio.example/" + key, nil
}

func wav() []byte {
	b := make([]byte, 128)
	copy(b, "RIFF")
	copy(b[8:], "WAVEfmt ")
	return b
}

func TestUploadIsolationValidationAndQuotas(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	objects := &fakeObjects{}
	s := New(repo, objects, time.Now, 1024, 256)
	song, err := s.Upload(ctx, "abc123", "../my song.wav", bytes.NewReader(wav()))
	if err != nil {
		t.Fatal(err)
	}
	if song.Name != "my song.wav" || song.Room != "abc123" || song.Size != 128 {
		t.Fatal(song)
	}
	if song.URL == "" || len(objects.objects) != 1 {
		t.Fatal("missing object or URL")
	}
	for _, input := range [][]byte{[]byte("<html>not audio"), make([]byte, 1025)} {
		if _, err := s.Upload(ctx, "abc123", "file.mp3", bytes.NewReader(input)); !errors.Is(err, ErrUpload) {
			t.Fatal(err)
		}
	}
	if _, err := s.Get(ctx, "other", song.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-room access", err)
	}
	if _, err := s.Upload(ctx, "abc123", "another.wav", bytes.NewReader(wav())); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upload(ctx, "abc123", "full.wav", bytes.NewReader(wav())); !errors.Is(err, ErrQuota) {
		t.Fatal(err)
	}
	list, err := s.List(ctx, "other")
	if err != nil || len(list) != 0 {
		t.Fatalf("%v %v", list, err)
	}
}

func TestCleanupRetriesAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	repo := NewMemory()
	objects := &fakeObjects{}
	s := New(repo, objects, func() time.Time { return now }, 1024, 4096)
	song, err := s.Upload(ctx, "abc123", "song.wav", bytes.NewReader(wav()))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(7 * time.Hour)
	objects.failDelete = true
	if err := s.Cleanup(ctx, nil); err == nil {
		t.Fatal("expected delete failure")
	}
	if len(objects.objects) != 1 {
		t.Fatal("unexpected delete")
	}
	objects.failDelete = false
	// A new service can find expired files in durable metadata without old rooms.
	restarted := New(repo, objects, func() time.Time { return now }, 1024, 4096)
	if err := restarted.Cleanup(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(objects.objects) != 0 {
		t.Fatal("leaked object")
	}
	if _, err := repo.Get(ctx, song.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestFailedUploadCannotAppearInPlaylist(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	objects := &fakeObjects{failPut: true, failDelete: true}
	s := New(repo, objects, time.Now, 1024, 4096)
	if _, err := s.Upload(ctx, "abc123", "song.wav", bytes.NewReader(wav())); err == nil {
		t.Fatal("expected upload failure")
	}
	songs, err := s.List(ctx, "abc123")
	if err != nil || len(songs) != 0 {
		t.Fatal("incomplete song visible", songs, err)
	}
	objects.failDelete = false
	if err := s.Cleanup(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	if size, _ := repo.TotalSize(ctx); size != 0 {
		t.Fatal("leaked metadata")
	}
}

func TestRoomCleanupPreservesDefaultsAndOtherRooms(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	objects := &fakeObjects{}
	s := New(repo, objects, time.Now, 1024, 4096)
	for _, room := range []string{"", "abc123", "def456"} {
		if _, err := s.Upload(ctx, room, "song.wav", bytes.NewReader(wav())); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Cleanup(ctx, []string{"abc123"}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.List(ctx, "abc123")
	b, _ := s.List(ctx, "def456")
	if len(a) != 1 || len(b) != 2 || len(objects.objects) != 2 {
		t.Fatalf("%d %d %d", len(a), len(b), len(objects.objects))
	}
}

func TestDetectSupportedAudio(t *testing.T) {
	for _, data := range [][]byte{wav(), append([]byte("ID3"), make([]byte, 40)...), append([]byte("OggS"), make([]byte, 40)...)} {
		if _, err := AudioType(data); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AudioType([]byte("RIFF1234WEBP")); err == nil {
		t.Fatal("accepted image as WAV")
	}
}

func TestConcurrentUploadsCannotExceedStorageQuota(t *testing.T) {
	s := New(NewMemory(), &fakeObjects{}, time.Now, 1024, 128)
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_, err := s.Upload(context.Background(), "abc123", "song.wav", bytes.NewReader(wav()))
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(err, ErrQuota) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if successes != 1 {
		t.Fatal("quota reservation raced", successes)
	}
}
