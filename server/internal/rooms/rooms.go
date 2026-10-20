// Package rooms owns ephemeral room state. All mutations are serialized so
// playback revisions have a single, deterministic order across concurrent peers.
package rooms

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	Lifetime    = 6 * time.Hour
	EmptyGrace  = 2 * time.Minute
	MaxRooms    = 100
	MaxMembers  = 20
	MaxPosition = 6 * 60 * 60
)

var (
	ErrRoom     = errors.New("room does not exist or has expired")
	ErrSession  = errors.New("session has expired; please join a room again")
	ErrCapacity = errors.New("room or server capacity reached")
	ErrConflict = errors.New("playback changed; using the latest room state")
	ErrPosition = errors.New("seek position must be between 0 and 21600 seconds")
	ErrNoSong   = errors.New("select a song first")
)

type Playback struct {
	SongID    string  `json:"songId"`
	Position  float64 `json:"position"`
	UpdatedAt int64   `json:"updatedAt"`
	Revision  uint64  `json:"revision"`
}

type Member struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

type Snapshot struct {
	Code       string   `json:"code"`
	Members    []Member `json:"members"`
	Playback   Playback `json:"playback"`
	ServerTime int64    `json:"serverTime"`
	ExpiresAt  int64    `json:"expiresAt"`
}

type Session struct {
	Token    string   `json:"token"`
	MemberID string   `json:"memberId"`
	Room     Snapshot `json:"room"`
}

type participant struct {
	Member
	updates chan string
}

type room struct {
	code       string
	created    time.Time
	emptySince time.Time
	playback   Playback
	members    map[[32]byte]*participant
}

type Manager struct {
	mu     sync.Mutex
	now    func() time.Time
	rooms  map[string]*room
	tokens map[[32]byte]*room
}

func New(now func() time.Time) *Manager {
	return &Manager{now: now, rooms: make(map[string]*room), tokens: make(map[[32]byte]*room)}
}

func randomID(bytes int) string {
	b := make([]byte, bytes)
	// crypto/rand.Read terminates the process on an unrecoverable entropy failure.
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) Create() (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rooms) >= MaxRooms {
		return Session{}, ErrCapacity
	}
	code := randomID(3)
	for m.rooms[code] != nil {
		code = randomID(3)
	}
	now := m.now()
	r := &room{code: code, created: now, emptySince: now, members: make(map[[32]byte]*participant)}
	m.rooms[code] = r
	return m.add(r), nil
}

func (m *Manager) Join(code string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rooms[code]
	if r == nil || m.expired(r) {
		return Session{}, ErrRoom
	}
	if len(r.members) >= MaxMembers {
		return Session{}, ErrCapacity
	}
	return m.add(r), nil
}

func (m *Manager) add(r *room) Session {
	token := randomID(32)
	hash := sha256.Sum256([]byte(token))
	id := randomID(8)
	adjectives := []string{"Mellow", "Sunny", "Cosmic", "Groovy", "Velvet", "Electric", "Dreamy", "Golden"}
	animals := []string{"Otter", "Panda", "Fox", "Owl", "Koala", "Tiger", "Robin", "Lynx"}
	name := adjectives[len(r.members)%len(adjectives)] + " " + animals[int(id[0])%len(animals)]
	r.members[hash] = &participant{Member: Member{ID: id, Name: name}}
	m.tokens[hash] = r
	m.notify(r, "snapshot", nil)
	return Session{Token: token, MemberID: id, Room: m.snapshot(r)}
}

func (m *Manager) expired(r *room) bool {
	now := m.now()
	return !now.Before(r.created.Add(Lifetime)) || (!r.emptySince.IsZero() && !now.Before(r.emptySince.Add(EmptyGrace)))
}

func (m *Manager) find(token string) (*room, [32]byte, error) {
	hash := sha256.Sum256([]byte(token))
	r := m.tokens[hash]
	if r == nil || m.expired(r) {
		return nil, hash, ErrSession
	}
	return r, hash, nil
}

func (m *Manager) snapshot(r *room) Snapshot {
	now := m.now()
	p := r.playback
	if p.SongID != "" {
		p.Position = math.Max(0, p.Position+float64(now.UnixMilli()-p.UpdatedAt)/1000)
	}
	p.UpdatedAt = now.UnixMilli()
	members := make([]Member, 0, len(r.members))
	for _, member := range r.members {
		members = append(members, member.Member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return Snapshot{Code: r.code, Members: members, Playback: p, ServerTime: now.UnixMilli(), ExpiresAt: r.created.Add(Lifetime).UnixMilli()}
}

func (m *Manager) Snapshot(token string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, _, err := m.find(token)
	if err != nil {
		return Snapshot{}, err
	}
	return m.snapshot(r), nil
}

func (m *Manager) Select(token, songID string, revision uint64) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, _, err := m.find(token)
	if err != nil {
		return Snapshot{}, err
	}
	if r.playback.Revision != revision {
		return m.snapshot(r), ErrConflict
	}
	if songID == "" {
		return m.snapshot(r), ErrNoSong
	}
	r.playback = Playback{SongID: songID, UpdatedAt: m.now().UnixMilli(), Revision: revision + 1}
	m.notify(r, "snapshot", nil)
	return m.snapshot(r), nil
}

func (m *Manager) Seek(token string, position float64, revision uint64) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, _, err := m.find(token)
	if err != nil {
		return Snapshot{}, err
	}
	if r.playback.Revision != revision {
		return m.snapshot(r), ErrConflict
	}
	if r.playback.SongID == "" {
		return m.snapshot(r), ErrNoSong
	}
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 || position > MaxPosition {
		return m.snapshot(r), ErrPosition
	}
	r.playback.Position, r.playback.UpdatedAt, r.playback.Revision = position, m.now().UnixMilli(), revision+1
	m.notify(r, "snapshot", nil)
	return m.snapshot(r), nil
}

// Subscribe replaces the previous transport for this session. detach is
// idempotent and cannot disconnect a newer connection that has taken its place.
func (m *Manager) Subscribe(token string) (<-chan string, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, hash, err := m.find(token)
	if err != nil {
		return nil, nil, err
	}
	p := r.members[hash]
	if p.updates != nil {
		close(p.updates)
	}
	updates := make(chan string, 16)
	p.updates, p.Connected = updates, true
	r.emptySince = time.Time{}
	m.notify(r, "snapshot", p)
	detach := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if p.updates != updates {
			return
		}
		close(updates)
		p.updates, p.Connected = nil, false
		m.markEmpty(r)
		m.notify(r, "snapshot", p)
	}
	return updates, detach, nil
}

func (m *Manager) markEmpty(r *room) {
	for _, p := range r.members {
		if p.Connected {
			return
		}
	}
	if r.emptySince.IsZero() {
		r.emptySince = m.now()
	}
}

func (m *Manager) notify(r *room, event string, except *participant) {
	for _, p := range r.members {
		if p == except || p.updates == nil {
			continue
		}
		select {
		case p.updates <- event:
		default:
			// A slow peer must reconnect and fetch a complete snapshot.
			close(p.updates)
			p.updates, p.Connected = nil, false
		}
	}
	m.markEmpty(r)
}

func (m *Manager) NotifyPlaylist(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, _, err := m.find(token); err == nil {
		m.notify(r, "playlist", nil)
	}
}

func (m *Manager) Leave(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, hash, err := m.find(token)
	if err != nil {
		return err
	}
	p := r.members[hash]
	if p.updates != nil {
		close(p.updates)
		p.updates = nil
	}
	delete(r.members, hash)
	delete(m.tokens, hash)
	m.markEmpty(r)
	m.notify(r, "snapshot", nil)
	return nil
}

func (m *Manager) Expire() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var codes []string
	for code, r := range m.rooms {
		if !m.expired(r) {
			continue
		}
		for hash, p := range r.members {
			delete(m.tokens, hash)
			if p.updates != nil {
				close(p.updates)
				p.updates = nil
			}
		}
		delete(m.rooms, code)
		codes = append(codes, code)
	}
	return codes
}
