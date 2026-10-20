// Package catalog coordinates metadata and audio objects without coupling room
// state to a storage provider. Pending rows make failed uploads recoverable.
package catalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"

	"musicmirror/server/internal/rooms"
)

var (
	ErrNotFound = errors.New("song not found")
	ErrUpload   = errors.New("upload a valid MP3, WAV or OGG within the file size limit")
	ErrQuota    = errors.New("upload quota reached; try again after room songs expire")
)

type Song struct {
	ID          string     `json:"id"`
	Room        string     `json:"-"`
	Name        string     `json:"name"`
	ObjectKey   string     `json:"-"`
	ContentType string     `json:"contentType"`
	Size        int64      `json:"size"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"-"`
	Ready       bool       `json:"-"`
	URL         string     `json:"url"`
	Default     bool       `json:"default"`
}

type Repository interface {
	Create(context.Context, Song) error
	MarkReady(context.Context, string) error
	Get(context.Context, string) (Song, error)
	List(context.Context, string, time.Time) ([]Song, error)
	TotalSize(context.Context) (int64, error)
	ExpireRooms(context.Context, []string, time.Time) error
	Expired(context.Context, time.Time) ([]Song, error)
	Delete(context.Context, string) error
	Ping(context.Context) error
}

type Objects interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Delete(context.Context, string) error
	URL(context.Context, string) (string, error)
}

type Service struct {
	repo                Repository
	objects             Objects
	now                 func() time.Time
	maxUpload, maxTotal int64
	// One process is the deployment contract. Serialize upload reservations and
	// cleanup to prevent quota races and deleting objects while they are uploaded.
	mu sync.Mutex
}

func New(repo Repository, objects Objects, now func() time.Time, maxUpload, maxTotal int64) *Service {
	return &Service{repo: repo, objects: objects, now: now, maxUpload: maxUpload, maxTotal: maxTotal}
}

func AudioType(data []byte) (string, error) {
	typ := http.DetectContentType(data)
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return "audio/wav", nil
	}
	if typ == "audio/mpeg" {
		return typ, nil
	}
	// Ogg is a container: reject video codecs and require the supported signature.
	if len(data) >= 4 && string(data[:4]) == "OggS" && !bytes.Contains(data, []byte("theora")) {
		return "audio/ogg", nil
	}
	return "", ErrUpload
}

func (s *Service) Upload(ctx context.Context, room, name string, input io.Reader) (Song, error) {
	data, err := io.ReadAll(io.LimitReader(input, s.maxUpload+1))
	if err != nil {
		return Song{}, fmt.Errorf("read upload: %w", err)
	}
	if int64(len(data)) > s.maxUpload {
		return Song{}, ErrUpload
	}
	typ, err := AudioType(data)
	if err != nil {
		return Song{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	total, err := s.repo.TotalSize(ctx)
	if err != nil {
		return Song{}, err
	}
	if total+int64(len(data)) > s.maxTotal {
		return Song{}, ErrQuota
	}
	list, err := s.repo.List(ctx, room, s.now())
	if err != nil {
		return Song{}, err
	}
	count := 0
	for _, song := range list {
		if song.Room == room {
			count++
		}
	}
	if count >= 20 {
		return Song{}, ErrQuota
	}
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = string([]rune(name)[:min(len([]rune(name)), 120)])
	if strings.TrimSpace(name) == "" || name == "." {
		name = "Untitled audio"
	}
	var id [16]byte
	rand.Read(id[:])
	now := s.now()
	expires := now.Add(rooms.Lifetime)
	song := Song{ID: hex.EncodeToString(id[:]), Room: room, Name: name, ContentType: typ, Size: int64(len(data)), CreatedAt: now, ExpiresAt: &expires, Default: room == ""}
	song.ObjectKey = "rooms/" + room + "/" + song.ID
	if room == "" {
		song.ObjectKey = "defaults/" + song.ID
	}
	// Pending objects expire promptly if upload or commit fails, even defaults.
	pending := now.Add(10 * time.Minute)
	song.ExpiresAt = &pending
	if err = s.repo.Create(ctx, song); err != nil {
		return Song{}, err
	}
	if err = s.objects.Put(ctx, song.ObjectKey, bytes.NewReader(data), song.Size, typ); err != nil {
		return Song{}, fmt.Errorf("store audio: %w", err)
	}
	if err = s.repo.MarkReady(ctx, song.ID); err != nil {
		return Song{}, fmt.Errorf("commit audio: %w", err)
	}
	song.Ready = true
	song.ExpiresAt = &expires
	if room == "" {
		song.ExpiresAt = nil
	}
	song.URL, err = s.objects.URL(ctx, song.ObjectKey)
	return song, err
}

func (s *Service) Get(ctx context.Context, room, id string) (Song, error) {
	song, err := s.repo.Get(ctx, id)
	if err != nil {
		return Song{}, err
	}
	if !song.Ready || (song.Room != "" && song.Room != room) || (song.ExpiresAt != nil && !s.now().Before(*song.ExpiresAt)) {
		return Song{}, ErrNotFound
	}
	return song, nil
}

func (s *Service) List(ctx context.Context, room string) ([]Song, error) {
	songs, err := s.repo.List(ctx, room, s.now())
	if err != nil {
		return nil, err
	}
	for i := range songs {
		songs[i].URL, err = s.objects.URL(ctx, songs[i].ObjectKey)
		if err != nil {
			return nil, fmt.Errorf("sign playback URL: %w", err)
		}
	}
	return songs, nil
}

func (s *Service) Cleanup(ctx context.Context, expiredRooms []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.ExpireRooms(ctx, expiredRooms, s.now()); err != nil {
		return err
	}
	songs, err := s.repo.Expired(ctx, s.now())
	if err != nil {
		return err
	}
	var failures []error
	for _, song := range songs {
		if err := s.objects.Delete(ctx, song.ObjectKey); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := s.repo.Delete(ctx, song.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) Ping(ctx context.Context) error { return s.repo.Ping(ctx) }
