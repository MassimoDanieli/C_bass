//go:build !darwin && !windows

package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plan names the file to update from, for the program installed from the archive in the
// home folder; "" for one installed by the package manager, which updates it in its own way.
func Plan(exe string) string {
	if strings.HasPrefix(exe, "/usr/") || strings.HasPrefix(exe, "/opt/") || !writable(filepath.Dir(exe)) {
		return ""
	}
	return "C_bass-linux-x64.tar.gz"
}

func leftovers(exe string) []string {
	return []string{filepath.Join(filepath.Dir(exe), ".c_bass-update-*")}
}

// Apply takes the program out of the archive and puts it in place of this one, which Linux
// lets be replaced while it runs, and gives what starts it.
func Apply(asset Asset, exe string, progress func(done, total int64)) (func() error, error) {
	folder := filepath.Dir(exe)
	archive, err := download(asset, folder, progress)
	if err != nil {
		return nil, err
	}
	defer os.Remove(archive)
	fresh, err := unpack(archive, "C_bass-linux-x64/c-bass", folder)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(fresh, exe); err != nil {
		os.Remove(fresh)
		return nil, err
	}
	return func() error { return exec.Command(exe).Start() }, nil
}

// unpack takes one file out of a .tar.gz into a new file in a folder.
func unpack(archive, name, folder string) (string, error) {
	file, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer file.Close()
	packed, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	reader := tar.NewReader(packed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("the archive holds no %s", name)
		}
		if err != nil {
			return "", err
		}
		if header.Name != name && strings.TrimPrefix(header.Name, "./") != name {
			continue
		}
		out, err := os.CreateTemp(folder, ".c_bass-update-")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, io.LimitReader(reader, 200<<20)); err != nil {
			out.Close()
			os.Remove(out.Name())
			return "", err
		}
		out.Close()
		os.Chmod(out.Name(), 0o755)
		return out.Name(), nil
	}
}
