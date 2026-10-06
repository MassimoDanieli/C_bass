// Package provision finds the two large pieces the separation needs and that are not part of
// the program: the ONNX Runtime library and the Demucs model. They are looked for where the
// user points, then beside the program, then in the user's cache; failing that they are
// downloaded once, checked against a known hash, and kept in the cache.
package provision

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	runtimeVersion = "1.29.0"
	// ModelURL is where the model is published: the site of Manico, which serves the same file to browsers.
	ModelURL    = "https://basso.massimodanieli.com/assets/separator/htdemucs.onnx"
	modelSHA256 = "da9e5101ee0804d04933974b59d8aae9c862e80e14f2f24e7c74cae76bdbe748"
)

// release describes the official ONNX Runtime archive for one platform.
type release struct {
	archive, sha256, member, name string
}

var releases = map[string]release{
	"darwin/arm64":  {"onnxruntime-osx-arm64-" + runtimeVersion + ".tgz", "d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92", "lib/libonnxruntime." + runtimeVersion + ".dylib", "libonnxruntime.dylib"},
	"linux/amd64":   {"onnxruntime-linux-x64-" + runtimeVersion + ".tgz", "c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba", "lib/libonnxruntime.so." + runtimeVersion, "libonnxruntime.so"},
	"windows/amd64": {"onnxruntime-win-x64-" + runtimeVersion + ".zip", "c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2", "lib/onnxruntime.dll", "onnxruntime.dll"},
}

// Progress reports a download: what is being fetched, bytes so far and the total (0 if unknown).
type Progress func(what string, done, total int64)

// CacheDir is where downloads are kept.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "cbass")
	return dir, os.MkdirAll(dir, 0o755)
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// besideProgram lists places next to the executable where a bundled file may be: the same
// folder, and inside a macOS application bundle its Frameworks and Resources folders.
func besideProgram(name string) []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	return []string{
		filepath.Join(dir, name),
		filepath.Join(dir, "..", "Frameworks", name),
		filepath.Join(dir, "..", "Resources", name),
	}
}

// Runtime returns the path of the ONNX Runtime library, downloading it if need be.
func Runtime(progress Progress) (string, error) {
	if path := os.Getenv("CBASS_ORT_LIB"); path != "" {
		return path, nil
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	rel, ok := releases[platform]
	if !ok {
		return "", fmt.Errorf("no ONNX Runtime build is known for %s: set CBASS_ORT_LIB to the library", platform)
	}
	for _, path := range besideProgram(rel.name) {
		if exists(path) {
			return path, nil
		}
	}
	cache, err := CacheDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(cache, "onnxruntime-"+runtimeVersion, rel.name)
	if exists(target) {
		return target, nil
	}
	url := "https://github.com/microsoft/onnxruntime/releases/download/v" + runtimeVersion + "/" + rel.archive
	archive, err := fetch(url, rel.sha256, "ONNX Runtime", progress)
	if err != nil {
		return "", err
	}
	library, err := extract(archive, rel.archive, rel.member)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, writeAtomically(target, library)
}

// Model returns the path of the Demucs model, downloading it if need be.
func Model(progress Progress) (string, error) {
	if path := os.Getenv("CBASS_MODEL"); path != "" {
		return path, nil
	}
	for _, path := range besideProgram("htdemucs.onnx") {
		if exists(path) {
			return path, nil
		}
	}
	cache, err := CacheDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(cache, "htdemucs.onnx")
	if exists(target) {
		return target, nil
	}
	data, err := fetch(ModelURL, modelSHA256, "Demucs model", progress)
	if err != nil {
		return "", err
	}
	return target, writeAtomically(target, data)
}

func writeAtomically(path string, data []byte) error {
	temporary := path + ".part"
	if err := os.WriteFile(temporary, data, 0o755); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func fetch(url, wantSHA, what string, progress Progress) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", what, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s answered %s", what, url, response.Status)
	}
	var buffer bytes.Buffer
	if response.ContentLength > 0 {
		buffer.Grow(int(response.ContentLength))
	}
	chunk := make([]byte, 1<<20)
	var done int64
	for {
		n, err := response.Body.Read(chunk)
		buffer.Write(chunk[:n])
		done += int64(n)
		if progress != nil {
			progress(what, done, response.ContentLength)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("downloading %s: %w", what, err)
		}
	}
	sum := sha256.Sum256(buffer.Bytes())
	if got := hex.EncodeToString(sum[:]); got != wantSHA {
		return nil, fmt.Errorf("%s: the download does not match the expected file (sha256 %s)", what, got)
	}
	return buffer.Bytes(), nil
}

// extract pulls one file out of a .tgz or .zip archive; member is its path below the top folder.
func extract(archive []byte, name, member string) ([]byte, error) {
	matches := func(path string) bool {
		path = strings.TrimPrefix(filepath.ToSlash(path), "./")
		if i := strings.Index(path, "/"); i >= 0 {
			return path[i+1:] == member
		}
		return false
	}
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if matches(file.Name) {
				content, err := file.Open()
				if err != nil {
					return nil, err
				}
				defer content.Close()
				return io.ReadAll(content)
			}
		}
		return nil, fmt.Errorf("%s is not in %s", member, name)
	}
	unzipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s is not in %s", member, name)
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg && matches(header.Name) {
			return io.ReadAll(reader)
		}
	}
}
