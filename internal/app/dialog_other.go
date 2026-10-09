//go:build !darwin && !windows

package app

import (
	"os/exec"
)

const audioFiles = "*.mp3 *.wav *.flac *.m4a *.aac *.ogg *.opus *.aif *.aiff"

// helper runs whichever of the usual helpers for choosing files is installed.
func helper(commands [][]string) []string {
	for _, command := range commands {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		return lines(exec.Command(command[0], command[1:]...).Output())
	}
	return nil
}

// chooseFiles asks for files, several if wanted; nil if nothing was chosen.
func chooseFiles(prompt string) []string {
	return helper([][]string{
		{"zenity", "--file-selection", "--multiple", "--separator=\n", "--title", prompt, "--file-filter", "Audio | " + audioFiles + " *.MP3 *.WAV *.FLAC *.M4A"},
		{"kdialog", "--title", prompt, "--multiple", "--separate-output", "--getopenfilename", ".", audioFiles},
	})
}

// chooseFolder asks for a folder; "" if none was chosen.
func chooseFolder(prompt string) string {
	found := helper([][]string{
		{"zenity", "--file-selection", "--directory", "--title", prompt},
		{"kdialog", "--title", prompt, "--getexistingdirectory", "."},
	})
	if len(found) > 0 {
		return found[0]
	}
	return ""
}

// askText asks for a line of text; "" if nothing was given.
func askText(prompt string) string {
	found := helper([][]string{
		{"zenity", "--entry", "--title", "C_bass", "--text", prompt},
		{"kdialog", "--title", "C_bass", "--inputbox", prompt},
	})
	if len(found) > 0 {
		return found[0]
	}
	return ""
}
