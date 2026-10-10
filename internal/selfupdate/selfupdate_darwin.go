package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// bundle is the application the program runs in: …/C_bass.app for …/C_bass.app/Contents/MacOS/C_bass.
func bundle(exe string) (string, bool) {
	macOS := filepath.Dir(exe)
	contents := filepath.Dir(macOS)
	app := filepath.Dir(contents)
	ok := filepath.Base(macOS) == "MacOS" && filepath.Base(contents) == "Contents" && strings.HasSuffix(app, ".app")
	return app, ok
}

// Plan names the file to update from, or "" when the program cannot replace itself: not
// in an application, opened from a disk image, or moved by macOS to a place of its own
// because it was never moved out of Downloads (App Translocation).
func Plan(exe string) string {
	app, ok := bundle(exe)
	if !ok || strings.Contains(app, "/AppTranslocation/") || strings.HasPrefix(app, "/Volumes/") || !writable(filepath.Dir(app)) {
		return ""
	}
	return "C_bass-macos-arm64.zip"
}

func leftovers(exe string) []string {
	app, ok := bundle(exe)
	if !ok {
		return nil
	}
	return []string{filepath.Join(filepath.Dir(app), ".c_bass-old-*"), filepath.Join(filepath.Dir(app), ".c_bass-update-*")}
}

// Apply puts the new application where this one is, keeping its name, and gives what starts it.
func Apply(asset Asset, exe string, progress func(done, total int64)) (func() error, error) {
	app, ok := bundle(exe)
	if !ok {
		return nil, fmt.Errorf("not inside an application")
	}
	folder := filepath.Dir(app)
	archive, err := download(asset, folder, progress)
	if err != nil {
		return nil, err
	}
	defer os.Remove(archive)
	unpacked, err := os.MkdirTemp(folder, ".c_bass-update-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(unpacked)
	// ditto keeps what an application needs: its signature, its links, its permissions
	if out, err := exec.Command("ditto", "-x", "-k", archive, unpacked).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("unpacking: %v %s", err, out)
	}
	fresh := filepath.Join(unpacked, "C_bass.app")
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS", "C_bass")); err != nil {
		return nil, fmt.Errorf("the archive holds no application")
	}
	old := filepath.Join(folder, fmt.Sprintf(".c_bass-old-%d.app", time.Now().UnixNano()))
	if err := os.Rename(app, old); err != nil {
		return nil, err
	}
	if err := os.Rename(fresh, app); err != nil {
		os.Rename(old, app) // put back what was there
		return nil, err
	}
	return func() error { return exec.Command("open", "-n", app).Start() }, nil
}
