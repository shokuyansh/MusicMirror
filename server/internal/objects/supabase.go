package objects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type signedURL struct {
	url     string
	expires time.Time
}
type Supabase struct {
	base, key, bucket string
	client            *http.Client
	mu                sync.Mutex
	urls              map[string]signedURL
}

func NewSupabase(base, key, bucket string, client *http.Client) *Supabase {
	return &Supabase{base: strings.TrimRight(base, "/") + "/storage/v1", key: key, bucket: bucket, client: client, urls: map[string]signedURL{}}
}

func (s *Supabase) request(ctx context.Context, method, path string, body io.Reader, typ string, size int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", s.key)
	// Legacy service_role keys are JWTs; new sb_secret_ keys use apikey only.
	if !strings.HasPrefix(s.key, "sb_secret_") {
		req.Header.Set("Authorization", "Bearer "+s.key)
	}
	req.Header.Set("Content-Type", typ)
	if size >= 0 {
		req.ContentLength = size
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage request failed: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("storage returned HTTP %d", res.StatusCode)
	}
	return data, nil
}
func (s *Supabase) Put(ctx context.Context, key string, r io.Reader, size int64, typ string) error {
	if !safeKey.MatchString(key) {
		return fmt.Errorf("invalid object key")
	}
	_, err := s.request(ctx, "POST", "/object/"+url.PathEscape(s.bucket)+"/"+key, r, typ, size)
	return err
}
func (s *Supabase) Delete(ctx context.Context, key string) error {
	if !safeKey.MatchString(key) {
		return fmt.Errorf("invalid object key")
	}
	body, _ := json.Marshal(map[string]any{"prefixes": []string{key}})
	_, err := s.request(ctx, "DELETE", "/object/"+url.PathEscape(s.bucket), bytes.NewReader(body), "application/json", int64(len(body)))
	if err == nil {
		s.mu.Lock()
		delete(s.urls, key)
		s.mu.Unlock()
	}
	return err
}
func (s *Supabase) URL(ctx context.Context, key string) (string, error) {
	if !safeKey.MatchString(key) {
		return "", fmt.Errorf("invalid object key")
	}
	s.mu.Lock()
	cached, ok := s.urls[key]
	s.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.url, nil
	}
	body := []byte(`{"expiresIn":86400}`)
	data, err := s.request(ctx, "POST", "/object/sign/"+url.PathEscape(s.bucket)+"/"+key, bytes.NewReader(body), "application/json", int64(len(body)))
	if err != nil {
		return "", err
	}
	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	// Supabase returns a path relative to /storage/v1, never a user supplied URL.
	if !strings.HasPrefix(result.SignedURL, "/object/sign/") {
		return "", fmt.Errorf("invalid signed URL response")
	}
	playbackURL := s.base + result.SignedURL
	s.mu.Lock()
	// Bound cached grants even after long uptime and expired rooms.
	for k, v := range s.urls {
		if time.Now().After(v.expires) {
			delete(s.urls, k)
		}
	}
	s.urls[key] = signedURL{url: playbackURL, expires: time.Now().Add(12 * time.Hour)}
	s.mu.Unlock()
	return playbackURL, nil
}
