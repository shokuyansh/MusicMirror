package rooms

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

func fixture() (*Manager, *time.Time) {
	now := time.Unix(1700000000, 0)
	return New(func() time.Time { return now }), &now
}

func TestTimelineSeekAndLocalPauseSemantics(t *testing.T) {
	m, now := fixture()
	owner, err := m.Create()
	if err != nil {
		t.Fatal(err)
	}
	guest, err := m.Join(owner.Room.Code)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Select(owner.Token, "song-a", 0); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(10 * time.Second)
	state, _ := m.Snapshot(guest.Token)
	if state.Playback.Position != 10 {
		t.Fatalf("position = %v", state.Playback.Position)
	}
	if _, err = m.Seek(guest.Token, 42, 1); err != nil {
		t.Fatal(err)
	}
	// Local pause has no server command: elapsed room time continues for everyone.
	*now = now.Add(5 * time.Second)
	state, _ = m.Snapshot(owner.Token)
	if state.Playback.Position != 47 || state.Playback.Revision != 2 {
		t.Fatalf("state = %+v", state.Playback)
	}
	if state.Playback.UpdatedAt != now.UnixMilli() {
		t.Fatal("snapshot must be rebased to server time")
	}
}

func TestStaleAndInvalidCommandsDoNotMutateTimeline(t *testing.T) {
	m, _ := fixture()
	a, _ := m.Create()
	if _, err := m.Seek(a.Token, 1, 0); !errors.Is(err, ErrNoSong) {
		t.Fatalf("err = %v", err)
	}
	m.Select(a.Token, "song", 0)
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), MaxPosition + 1} {
		if _, err := m.Seek(a.Token, value, 1); !errors.Is(err, ErrPosition) {
			t.Fatalf("%v: %v", value, err)
		}
	}
	if _, err := m.Select(a.Token, "other", 0); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	state, _ := m.Snapshot(a.Token)
	if state.Playback.SongID != "song" || state.Playback.Revision != 1 {
		t.Fatal(state)
	}
	if _, err := m.Seek("bad-token", 5, 1); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
}

func TestConcurrentCommandsHaveExactlyOneWinner(t *testing.T) {
	m := New(time.Now)
	a, _ := m.Create()
	m.Select(a.Token, "song", 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < 40; i++ {
		wg.Go(func() {
			_, err := m.Seek(a.Token, 10, 1)
			if err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners = %d", winners)
	}
}

func TestConnectionsReplaceAndDisconnectSafely(t *testing.T) {
	m, _ := fixture()
	a, _ := m.Create()
	first, detachFirst, err := m.Subscribe(a.Token)
	if err != nil {
		t.Fatal(err)
	}
	second, detachSecond, err := m.Subscribe(a.Token)
	if err != nil {
		t.Fatal(err)
	}
	detachFirst()
	state, _ := m.Snapshot(a.Token)
	if !state.Members[0].Connected {
		t.Fatal("old disconnect disconnected new socket")
	}
	if _, open := <-first; open {
		t.Fatal("old subscription should be closed")
	}
	m.NotifyPlaylist(a.Token)
	select {
	case <-second:
	default:
		t.Fatal("new subscription did not get update")
	}
	detachSecond()
	detachSecond()
	state, _ = m.Snapshot(a.Token)
	if state.Members[0].Connected {
		t.Fatal("member remains connected")
	}
}

func TestExpiryGraceAndLeaveRevokeTokens(t *testing.T) {
	m, now := fixture()
	a, _ := m.Create()
	_, detach, _ := m.Subscribe(a.Token)
	detach()
	*now = now.Add(EmptyGrace - time.Second)
	if len(m.Expire()) != 0 {
		t.Fatal("expired too soon")
	}
	_, detach, _ = m.Subscribe(a.Token)
	*now = now.Add(2 * time.Second)
	if len(m.Expire()) != 0 {
		t.Fatal("expired occupied room")
	}
	detach()
	*now = now.Add(EmptyGrace)
	if got := m.Expire(); len(got) != 1 || got[0] != a.Room.Code {
		t.Fatal(got)
	}
	if _, err := m.Snapshot(a.Token); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
	if _, err := m.Join(a.Room.Code); !errors.Is(err, ErrRoom) {
		t.Fatal(err)
	}
	b, _ := m.Create()
	if err := m.Leave(b.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Snapshot(b.Token); !errors.Is(err, ErrSession) {
		t.Fatal(err)
	}
}

func TestRoomCapacityAndAbsoluteLifetime(t *testing.T) {
	m, now := fixture()
	a, _ := m.Create()
	for i := 1; i < MaxMembers; i++ {
		if _, err := m.Join(a.Room.Code); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Join(a.Room.Code); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	_, detach, _ := m.Subscribe(a.Token)
	defer detach()
	*now = now.Add(Lifetime)
	if _, err := m.Snapshot(a.Token); !errors.Is(err, ErrSession) {
		t.Fatal("expired token accepted", err)
	}
	if len(m.Expire()) != 1 {
		t.Fatal("absolute lifetime not enforced")
	}
}

func TestSnapshotsAreIndependentCopies(t *testing.T) {
	m, _ := fixture()
	a, _ := m.Create()
	one, _ := m.Snapshot(a.Token)
	one.Members[0].Name = "changed"
	two, _ := m.Snapshot(a.Token)
	if two.Members[0].Name == "changed" {
		t.Fatal("snapshot aliases internal state")
	}
}
