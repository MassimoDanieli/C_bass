package app

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func names(found []pending) []string {
	var out []string
	for _, item := range found {
		out = append(out, item.name)
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A folder gives its recordings, the ones in folders within it too, in the order of their
// names; what is not audio, and what the system hides, is left out.
func TestRecordingsInAFolder(t *testing.T) {
	files := fstest.MapFS{
		"b.mp3":            {Data: []byte("b")},
		"a.WAV":            {Data: []byte("a")},
		"cover.jpg":        {Data: []byte("x")},
		"._a.WAV":          {Data: []byte("x")},
		"live/c.flac":      {Data: []byte("c")},
		"live/notes.txt":   {Data: []byte("x")},
		".trash/gone.mp3":  {Data: []byte("x")},
		"live/deep/d.m4a":  {Data: []byte("d")},
		"empty/readme.txt": {Data: []byte("x")},
	}
	found := recordingsIn(files)
	if want := []string{"a.WAV", "b.mp3", "c.flac", "d.m4a"}; !same(names(found), want) {
		t.Fatalf("found %v, want %v", names(found), want)
	}
	for _, item := range found {
		data, err := item.read()
		if err != nil || len(data) != 1 {
			t.Fatalf("%s: read %q, %v", item.name, data, err)
		}
	}
}

// Paths can be files and folders together; one that is not there is passed over.
func TestRecordingsAtPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "album"), 0o755)
	for _, name := range []string{"one.mp3", "album/two.mp3", "album/three.wav", "album/cover.png"} {
		os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644)
	}
	found := recordingsAt([]string{filepath.Join(dir, "one.mp3"), filepath.Join(dir, "missing.mp3"), filepath.Join(dir, "album"), filepath.Join(dir, "album", "cover.png")})
	if want := []string{"one.mp3", "three.wav", "two.mp3"}; !same(names(found), want) {
		t.Fatalf("found %v, want %v", names(found), want)
	}
	if data, err := found[2].read(); err != nil || string(data) != "album/two.mp3" {
		t.Fatalf("read %q, %v", data, err)
	}
}

func TestLinesOfAHelper(t *testing.T) {
	if got := lines([]byte("/a/one.mp3\r\n/b/two three.wav\n\n"), nil); !same(got, []string{"/a/one.mp3", "/b/two three.wav"}) {
		t.Fatalf("got %q", got)
	}
	if got := lines([]byte("/a\n"), os.ErrNotExist); got != nil {
		t.Fatalf("a helper that failed chose nothing, got %q", got)
	}
}
