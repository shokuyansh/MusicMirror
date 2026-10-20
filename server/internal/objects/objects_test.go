package objects

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const objectKey = "rooms/abc123/0123456789abcdef0123456789abcdef"

func TestLocalSignedPlaybackRangesAndExpiry(t *testing.T) {
	now := time.Now()
	local, err := NewLocal(t.TempDir(), bytes.Repeat([]byte("a"), 32), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("ID3abcdefghijklmnopqrstuv")
	if err = local.Put(context.Background(), objectKey, bytes.NewReader(data), int64(len(data)), "audio/mpeg"); err != nil {
		t.Fatal(err)
	}
	u, _ := local.URL(context.Background(), objectKey)
	req := httptest.NewRequest("GET", u, nil)
	req.Header.Set("Range", "bytes=3-6")
	w := httptest.NewRecorder()
	local.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "abcd" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	for _, bad := range []string{strings.Replace(u, "signature=", "signature=x", 1), strings.Replace(u, "abc123", "abc124", 1), "/media/../../etc/passwd"} {
		w = httptest.NewRecorder()
		local.ServeHTTP(w, httptest.NewRequest("GET", bad, nil))
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	now = now.Add(24 * time.Hour)
	w = httptest.NewRecorder()
	local.ServeHTTP(w, httptest.NewRequest("GET", u, nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	if err = local.Delete(context.Background(), objectKey); err != nil {
		t.Fatal(err)
	}
	if err = local.Delete(context.Background(), objectKey); err != nil {
		t.Fatal("delete must be idempotent", err)
	}
	if err = local.Put(context.Background(), "../escape", bytes.NewReader(data), int64(len(data)), "audio/mpeg"); err == nil {
		t.Fatal("accepted path traversal")
	}
}

func TestSupabaseContractAndSecretIsolation(t *testing.T) {
	var uploaded, deleted, signed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "server-secret" || r.Header.Get("Authorization") != "Bearer server-secret" {
			t.Error("missing server authentication")
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/storage/v1/object/music/"+objectKey:
			data, _ := io.ReadAll(r.Body)
			if string(data) != "audio" || r.Header.Get("Content-Type") != "audio/mpeg" {
				t.Error("invalid upload")
			}
			uploaded = true
			w.Write([]byte(`{"Key":"music/key"}`))
		case r.Method == "POST" && r.URL.Path == "/storage/v1/object/sign/music/"+objectKey:
			var body struct {
				ExpiresIn int `json:"expiresIn"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.ExpiresIn != 86400 {
				t.Error(body)
			}
			signed = true
			w.Write([]byte(`{"signedURL":"/object/sign/music/key?token=temporary"}`))
		case r.Method == "DELETE" && r.URL.Path == "/storage/v1/object/music":
			var body struct {
				Prefixes []string `json:"prefixes"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Prefixes) != 1 || body.Prefixes[0] != objectKey {
				t.Error(body)
			}
			deleted = true
			w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	s := NewSupabase(server.URL, "server-secret", "music", server.Client())
	ctx := context.Background()
	if err := s.Put(ctx, objectKey, strings.NewReader("audio"), 5, "audio/mpeg"); err != nil {
		t.Fatal(err)
	}
	u, err := s.URL(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u, "server-secret") || u != server.URL+"/storage/v1/object/sign/music/key?token=temporary" {
		t.Fatal(u)
	}
	if err = s.Delete(ctx, objectKey); err != nil {
		t.Fatal(err)
	}
	if !uploaded || !signed || !deleted {
		t.Fatal("incomplete contract coverage")
	}
}

func TestSupabaseErrorsAreSafeAndRespectCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("sensitive upstream diagnostic"))
	}))
	defer server.Close()
	s := NewSupabase(server.URL, "secret", "music", server.Client())
	_, err := s.URL(context.Background(), objectKey)
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = s.Put(ctx, objectKey, strings.NewReader("audio"), 5, "audio/mpeg"); err == nil {
		t.Fatal("ignored cancellation")
	}
}
