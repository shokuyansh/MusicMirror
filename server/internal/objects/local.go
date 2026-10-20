// Package objects implements private audio storage. URL grants are temporary
// bearer capabilities; server storage credentials never reach the browser.
package objects

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var safeKey = regexp.MustCompile(`^(rooms/[a-f0-9]{6}|defaults)/[a-f0-9]{32}$`)

type Local struct {
	root   string
	secret []byte
	now    func() time.Time
}

func NewLocal(root string, secret []byte, now func() time.Time) (*Local, error) {
	if len(secret) < 32 {
		return nil, errors.New("local signing key must be at least 32 bytes")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Local{root: root, secret: secret, now: now}, nil
}
func (l *Local) filename(key string) (string, error) {
	if !safeKey.MatchString(key) {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(l.root, filepath.FromSlash(key)), nil
}
func (l *Local) Put(ctx context.Context, key string, r io.Reader, size int64, _ string) error {
	filename, err := l.filename(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".upload-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, err := io.Copy(f, r)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n != size {
		return errors.New("incomplete upload")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}
func (l *Local) Delete(_ context.Context, key string) error {
	filename, err := l.filename(key)
	if err != nil {
		return err
	}
	err = os.Remove(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (l *Local) signature(key, expires string) string {
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(key + "\n" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}
func (l *Local) URL(_ context.Context, key string) (string, error) {
	if _, err := l.filename(key); err != nil {
		return "", err
	}
	expires := strconv.FormatInt(l.now().Add(24*time.Hour).Unix(), 10)
	return "/media/" + key + "?" + url.Values{"expires": {expires}, "signature": {l.signature(key, expires)}}.Encode(), nil
}
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/media/")
	expires := r.URL.Query().Get("expires")
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || l.now().Unix() >= exp || !hmac.Equal([]byte(l.signature(key, expires)), []byte(r.URL.Query().Get("signature"))) {
		http.Error(w, "playback URL expired or invalid", http.StatusForbidden)
		return
	}
	filename, err := l.filename(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, key, stat.ModTime(), f)
}
