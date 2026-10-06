package app

import (
	"os/exec"
	"strings"
	"syscall"
)

// chooseFile shows the system's own window for choosing a file; "" if nothing was chosen.
func chooseFile(prompt string) string {
	script := `Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '` + strings.ReplaceAll(prompt, "'", "") + `'
$d.Filter = 'Audio|*.mp3;*.wav;*.flac;*.m4a;*.aac;*.ogg;*.opus;*.aif;*.aiff'
if ($d.ShowDialog() -eq 'OK') { Write-Output $d.FileName }`
	command := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}
