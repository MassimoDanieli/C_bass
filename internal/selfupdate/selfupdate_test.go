package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fingerprint(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

func TestFind(t *testing.T) {
	assets := []Asset{{Name: "a.zip", URL: "u", Digest: ""}, {Name: "b.zip", URL: "u", Digest: "sha256:00"}}
	if _, err := Find(assets, "a.zip"); err == nil {
		t.Fatal("a file without a fingerprint is not to be trusted")
	}
	if _, err := Find(assets, "b.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(assets, "c.zip"); err == nil {
		t.Fatal("a file that is not there")
	}
}

func TestDownloadChecksTheFingerprint(t *testing.T) {
	body := []byte(strings.Repeat("new version ", 50000))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	folder := t.TempDir()
	path, err := download(Asset{Name: "x.zip", URL: server.URL, Digest: fingerprint(body)}, folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, body) {
		t.Fatal("the file is not what was sent")
	}
	if _, err := download(Asset{Name: "x.zip", URL: server.URL, Digest: fingerprint([]byte("something else"))}, folder, nil); err == nil {
		t.Fatal("a file that does not match its fingerprint must be refused")
	}
	if left, _ := filepath.Glob(filepath.Join(folder, ".c_bass-update-*")); len(left) != 1 {
		t.Fatalf("a refused file must not stay behind: %v", left)
	}
}

// On Linux the program from the archive in the home folder is replaced by the new one in the archive.
func TestApplyOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}
	var archive bytes.Buffer
	zipped := gzip.NewWriter(&archive)
	packed := tar.NewWriter(zipped)
	for name, data := range map[string]string{"C_bass-linux-x64/install.sh": "#!/bin/sh", "C_bass-linux-x64/c-bass": "#!/bin/sh\necho new\n"} {
		packed.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))})
		packed.Write([]byte(data))
	}
	packed.Close()
	zipped.Close()
	body := archive.Bytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	bin := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(bin, 0o755)
	exe := filepath.Join(bin, "c-bass")
	os.WriteFile(exe, []byte("old"), 0o755)
	if Plan(exe) != "C_bass-linux-x64.tar.gz" || Plan("/usr/bin/c-bass") != "" {
		t.Fatal("plan")
	}
	start, err := Apply(Asset{Name: "C_bass-linux-x64.tar.gz", URL: server.URL, Digest: fingerprint(body)}, exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "#!/bin/sh\necho new\n" {
		t.Fatalf("got %q", got)
	}
	if info, _ := os.Stat(exe); info.Mode()&0o111 == 0 {
		t.Fatal("the new program must be runnable")
	}
	if start == nil {
		t.Fatal("nothing to start")
	}
	Cleanup(exe)
	if left, _ := filepath.Glob(filepath.Join(bin, ".c_bass-*")); len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}
