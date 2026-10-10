package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
)

// installed says whether the program was put in place by its installer, which leaves its uninstaller beside it.
func installed(exe string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe"))
	return err == nil
}

// Plan names the file to update from: the installer for a program it installed, the
// program alone for one run without installing; "" when the folder cannot be written to.
func Plan(exe string) string {
	if installed(exe) {
		return "C_bass-windows-x64-setup.exe"
	}
	if writable(filepath.Dir(exe)) {
		return "C_bass-windows-x64-portable.exe"
	}
	return ""
}

func leftovers(exe string) []string {
	return []string{exe + ".old", filepath.Join(filepath.Dir(exe), ".c_bass-update-*")}
}

// Apply readies the new version and gives what puts it in place and starts it: the installer,
// asked to work without questions and to start the program again; or, for the program run
// without installing, the new file in place of this one, which Windows lets be renamed while it runs.
func Apply(asset Asset, exe string, progress func(done, total int64)) (func() error, error) {
	if installed(exe) {
		setup, err := download(asset, os.TempDir(), progress)
		if err != nil {
			return nil, err
		}
		named := setup + ".exe"
		if err := os.Rename(setup, named); err != nil {
			return nil, err
		}
		return func() error {
			return exec.Command(named, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS", "/relaunch=yes").Start()
		}, nil
	}
	fresh, err := download(asset, filepath.Dir(exe), progress)
	if err != nil {
		return nil, err
	}
	os.Remove(exe + ".old")
	if err := os.Rename(exe, exe+".old"); err != nil {
		os.Remove(fresh)
		return nil, err
	}
	if err := os.Rename(fresh, exe); err != nil {
		os.Rename(exe+".old", exe)
		os.Remove(fresh)
		return nil, err
	}
	return func() error { return exec.Command(exe).Start() }, nil
}
