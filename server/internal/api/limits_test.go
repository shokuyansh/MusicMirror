package api

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimitsAndTrustedProxyAddress(t *testing.T) {
	l := limiter{entries: map[string]window{}}
	if !l.allow("test", 2, time.Minute) || !l.allow("test", 2, time.Minute) || l.allow("test", 2, time.Minute) {
		t.Fatal("rate limit not enforced")
	}
	a, _ := testAPI(t)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if a.ip(r) != "127.0.0.1" {
		t.Fatal("trusted unconfigured proxy header")
	}
	a.opts.TrustProxy = true
	if a.ip(r) != "5.6.7.8" {
		t.Fatal("must use last proxy-appended address")
	}
	for i := 0; i < 20; i++ {
		request(t, a.Handler(), "POST", "/api/rooms", "", "{}")
	}
	if w := request(t, a.Handler(), "POST", "/api/rooms", "", "{}"); w.Code != 429 {
		t.Fatal(w.Code)
	}
}

func TestUploadRejectsUnexpectedFieldsNonAudioAndOversize(t *testing.T) {
	a, _ := testAPI(t)
	session := createSession(t, a)
	for _, tc := range []struct {
		name   string
		extra  bool
		data   []byte
		status int
	}{
		{"not audio", false, []byte("<html>no</html>"), 400},
		{"extra field", true, []byte("ID3audio"), 400},
		{"file limit", false, append([]byte("ID3"), make([]byte, 1024)...), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			file, _ := form.CreateFormFile("song", "file.mp3")
			file.Write(tc.data)
			if tc.extra {
				form.WriteField("room", "victim")
			}
			form.Close()
			r := httptest.NewRequest("POST", "/api/songs", &body)
			r.Header.Set("Authorization", "Bearer "+session.Token)
			r.Header.Set("Content-Type", form.FormDataContentType())
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
	r := httptest.NewRequest("POST", "/api/songs", strings.NewReader(strings.Repeat("x", (1<<20)+2048)))
	r.Header.Set("Authorization", "Bearer "+session.Token)
	r.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
}
