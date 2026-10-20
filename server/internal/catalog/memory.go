package catalog

import (
	"context"
	"sort"
	"sync"
	"time"

	"musicmirror/server/internal/rooms"
)

// Memory is the development/test repository. Production uses PostgreSQL.
type Memory struct {
	mu    sync.Mutex
	songs map[string]Song
}

func NewMemory() *Memory { return &Memory{songs: map[string]Song{}} }
func (m *Memory) Create(_ context.Context, song Song) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.songs[song.ID] = song
	return nil
}
func (m *Memory) MarkReady(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	song, ok := m.songs[id]
	if !ok {
		return ErrNotFound
	}
	song.Ready = true
	expires := song.CreatedAt.Add(rooms.Lifetime)
	song.ExpiresAt = &expires
	if song.Room == "" {
		song.ExpiresAt = nil
	}
	m.songs[id] = song
	return nil
}
func (m *Memory) Get(_ context.Context, id string) (Song, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.songs[id]
	if !ok {
		return Song{}, ErrNotFound
	}
	return s, nil
}
func (m *Memory) List(_ context.Context, room string, now time.Time) ([]Song, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	songs := []Song{}
	for _, song := range m.songs {
		if song.Ready && (song.Room == "" || song.Room == room) && (song.ExpiresAt == nil || now.Before(*song.ExpiresAt)) {
			songs = append(songs, song)
		}
	}
	sort.Slice(songs, func(i, j int) bool {
		if songs[i].Default != songs[j].Default {
			return songs[i].Default
		}
		if songs[i].CreatedAt.Equal(songs[j].CreatedAt) {
			return songs[i].ID < songs[j].ID
		}
		return songs[i].CreatedAt.Before(songs[j].CreatedAt)
	})
	return songs, nil
}
func (m *Memory) TotalSize(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, song := range m.songs {
		total += song.Size
	}
	return total, nil
}
func (m *Memory) ExpireRooms(_ context.Context, codes []string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, song := range m.songs {
		for _, code := range codes {
			if song.Room != "" && song.Room == code {
				song.ExpiresAt = &now
				m.songs[id] = song
			}
		}
	}
	return nil
}
func (m *Memory) Expired(_ context.Context, now time.Time) ([]Song, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	songs := []Song{}
	for _, song := range m.songs {
		if song.ExpiresAt != nil && !now.Before(*song.ExpiresAt) {
			songs = append(songs, song)
		}
	}
	return songs, nil
}
func (m *Memory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.songs, id)
	return nil
}
func (m *Memory) Ping(context.Context) error { return nil }
