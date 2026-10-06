//go:build !darwin && !windows

package app

import (
	"os/exec"
	"strings"
)

// chooseFile asks for a file with whichever of the usual helpers is installed; "" if nothing
// was chosen.
func chooseFile(prompt string) string {
	for _, command := range [][]string{
		{"zenity", "--file-selection", "--title", prompt, "--file-filter", "Audio | *.mp3 *.wav *.flac *.m4a *.aac *.ogg *.opus *.aif *.aiff *.MP3 *.WAV *.FLAC *.M4A"},
		{"kdialog", "--title", prompt, "--getopenfilename", ".", "*.mp3 *.wav *.flac *.m4a *.aac *.ogg *.opus *.aif *.aiff"},
	} {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		out, err := exec.Command(command[0], command[1:]...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimRight(string(out), "\r\n")
	}
	return ""
}
