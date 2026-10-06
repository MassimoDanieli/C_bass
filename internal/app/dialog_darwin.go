package app

import (
	"os/exec"
	"strings"
)

// chooseFile shows the system's own panel for choosing a file; "" if nothing was chosen.
func chooseFile(prompt string) string {
	// "activate" brings the panel in front of the window, which would otherwise hide it.
	script := `activate
set chosen to choose file with prompt "` + strings.ReplaceAll(prompt, `"`, "") + `" of type {"public.mp3", "com.microsoft.waveform-audio", "mp3", "wav"}
POSIX path of chosen`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}
