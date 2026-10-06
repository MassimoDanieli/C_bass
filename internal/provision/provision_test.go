package provision

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestExtract(t *testing.T) {
	var tgz bytes.Buffer
	zipped := gzip.NewWriter(&tgz)
	archive := tar.NewWriter(zipped)
	for name, body := range map[string]string{"./pkg-1.0/lib/other.so": "no", "./pkg-1.0/lib/libx.so.1.0": "library"} {
		archive.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		archive.Write([]byte(body))
	}
	archive.Close()
	zipped.Close()
	got, err := extract(tgz.Bytes(), "pkg.tgz", "lib/libx.so.1.0")
	if err != nil || string(got) != "library" {
		t.Fatalf("tgz: %q, %v", got, err)
	}

	var zipFile bytes.Buffer
	writer := zip.NewWriter(&zipFile)
	entry, _ := writer.Create("pkg-1.0/lib/x.dll")
	entry.Write([]byte("dll"))
	writer.Close()
	got, err = extract(zipFile.Bytes(), "pkg.zip", "lib/x.dll")
	if err != nil || string(got) != "dll" {
		t.Fatalf("zip: %q, %v", got, err)
	}
	if _, err := extract(zipFile.Bytes(), "pkg.zip", "lib/missing.dll"); err == nil {
		t.Fatal("a missing member should be an error")
	}
}

func TestEveryPlatformHasARelease(t *testing.T) {
	for _, platform := range []string{"darwin/arm64", "windows/amd64", "linux/amd64"} {
		if rel, ok := releases[platform]; !ok || len(rel.sha256) != 64 {
			t.Errorf("%s: no release, or no hash", platform)
		}
	}
}
