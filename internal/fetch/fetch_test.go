package fetch

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	for text, want := range map[string]error{
		"https://example.com/song.mp3":             nil,
		"  http://example.com/a/b.wav  ":           nil,
		"example.com/song.mp3":                     ErrNotALink,
		"ftp://example.com/song.mp3":               ErrNotALink,
		"song.mp3":                                 ErrNotALink,
		"https://www.youtube.com/watch?v=abc":      ErrVideoSite,
		"https://youtu.be/abc":                     ErrVideoSite,
		"https://music.youtube.com/watch?v=abc":    ErrVideoSite,
		"https://open.spotify.com/track/abc":       ErrVideoSite,
		"https://notyoutube.com.example.org/a.mp3": nil,
	} {
		if _, err := Check(text); !errors.Is(err, want) {
			t.Errorf("%q: got %v, want %v", text, err, want)
		}
	}
}

func serve(t *testing.T, handler http.HandlerFunc) string {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestGetAFile(t *testing.T) {
	body := strings.Repeat("x", 700<<10)
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/music/My%20Song.mp3", "/music/My Song.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write([]byte(body))
		case "/download":
			w.Header().Set("Content-Type", "audio/x-wav")
			w.Header().Set("Content-Disposition", `attachment; filename="take 2.wav"`)
			w.Write([]byte("RIFF....WAVE"))
		case "/stream":
			w.Header().Set("Content-Type", "audio/flac")
			w.Write([]byte("fLaC"))
		case "/moved":
			http.Redirect(w, r, "/music/My%20Song.mp3", http.StatusFound)
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<html></html>"))
		case "/sneaky":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("<!DOCTYPE html><html>"))
		default:
			http.NotFound(w, r)
		}
	})
	calls := 0
	data, name, err := Get(base+"/music/My%20Song.mp3", func(done, total int64) { calls++ })
	if err != nil || name != "My Song.mp3" || len(data) != len(body) || calls < 2 {
		t.Fatalf("got %q, %d bytes, %d reports, %v", name, len(data), calls, err)
	}
	for path, want := range map[string]string{"/download": "take 2.wav", "/stream": "stream.flac", "/moved": "My Song.mp3"} {
		if _, name, err := Get(base+path, nil); err != nil || name != want {
			t.Errorf("%s: got %q, %v; want %q", path, name, err, want)
		}
	}
	for path, want := range map[string]error{"/page": ErrNotAudio, "/sneaky": ErrNotAudio, "/missing.mp3": ErrNotReached} {
		if _, _, err := Get(base+path, nil); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", path, err, want)
		}
	}
}

func TestTooLarge(t *testing.T) {
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "999999999999")
	})
	if _, _, err := Get(base+"/big.mp3", nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}
