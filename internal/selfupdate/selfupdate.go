// Package selfupdate replaces the program with a newer version published on GitHub, from
// inside the program: the right file for this system is fetched, checked against the
// fingerprint GitHub gives for it, put in place of the one running, and started.
//
// Where the program cannot replace itself (installed by the system's package manager, run
// from a disk image, or from a folder it may not write to) Plan says so, and the window
// opens the page of the release instead.
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Asset is one file of a release.
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:…", as GitHub gives it
}

// ErrNoFile says the release has no file this program can update itself from.
var ErrNoFile = errors.New("the release has no file for this system")

// Find picks the file a plan wants out of a release, provided it comes with a fingerprint.
func Find(assets []Asset, name string) (Asset, error) {
	for _, asset := range assets {
		if asset.Name == name && strings.HasPrefix(asset.Digest, "sha256:") && asset.URL != "" {
			return asset, nil
		}
	}
	return Asset{}, ErrNoFile
}

// writable says whether files can be made in a folder.
func writable(folder string) bool {
	probe, err := os.CreateTemp(folder, ".c_bass-probe-")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

// download fetches a file into a new one in a folder, checking it against its fingerprint
// while it comes, and gives the path of what it made.
func download(asset Asset, folder string, progress func(done, total int64)) (string, error) {
	want, err := hex.DecodeString(strings.TrimPrefix(asset.Digest, "sha256:"))
	if err != nil || len(want) != sha256.Size {
		return "", fmt.Errorf("%s: the fingerprint cannot be read", asset.Name)
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	request, err := http.NewRequest(http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "C_bass")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", asset.Name, response.Status)
	}
	file, err := os.CreateTemp(folder, ".c_bass-update-*-"+asset.Name)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	var done int64
	buffer := make([]byte, 256<<10)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, err := file.Write(buffer[:n]); err != nil {
				file.Close()
				os.Remove(file.Name())
				return "", err
			}
			hash.Write(buffer[:n])
			done += int64(n)
			if progress != nil {
				progress(done, max(response.ContentLength, 0))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			file.Close()
			os.Remove(file.Name())
			return "", readErr
		}
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", err
	}
	if got := hash.Sum(nil); string(got) != string(want) {
		os.Remove(file.Name())
		return "", fmt.Errorf("%s: the file that came is not the one published (fingerprint %x)", asset.Name, got)
	}
	return file.Name(), nil
}

// Cleanup takes away what an earlier update left beside the program: the version replaced,
// and anything a broken update left half done.
func Cleanup(exe string) {
	for _, place := range leftovers(exe) {
		matches, _ := filepath.Glob(place)
		for _, match := range matches {
			os.RemoveAll(match)
		}
	}
}
