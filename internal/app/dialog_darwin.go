package app

import (
	"os/exec"
	"strings"
)

// chooseFiles shows the system's own panel for choosing files, several if wanted; nil if
// nothing was chosen.
func chooseFiles(prompt string) []string {
	// "activate" brings the panel in front of the window, which would otherwise hide it.
	script := `activate
set chosen to choose file with prompt "` + strings.ReplaceAll(prompt, `"`, "") + `" of type {"public.audio", "mp3", "wav", "flac", "m4a", "aac", "ogg", "opus", "aif", "aiff"} with multiple selections allowed
set answer to ""
repeat with one in chosen
	set answer to answer & POSIX path of one & linefeed
end repeat
answer`
	return lines(exec.Command("osascript", "-e", script).Output())
}

// chooseFolder shows the system's own panel for choosing a folder; "" if none was chosen.
func chooseFolder(prompt string) string {
	script := `activate
POSIX path of (choose folder with prompt "` + strings.ReplaceAll(prompt, `"`, "") + `")`
	if found := lines(exec.Command("osascript", "-e", script).Output()); len(found) > 0 {
		return found[0]
	}
	return ""
}
