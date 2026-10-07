package app

import (
	"os/exec"
	"strings"
	"syscall"
)

func powershell(script string) []string {
	command := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return lines(command.Output())
}

// chooseFiles shows the system's own window for choosing files, several if wanted; nil if
// nothing was chosen.
func chooseFiles(prompt string) []string {
	return powershell(`Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '` + strings.ReplaceAll(prompt, "'", "") + `'
$d.Filter = 'Audio|*.mp3;*.wav;*.flac;*.m4a;*.aac;*.ogg;*.opus;*.aif;*.aiff'
$d.Multiselect = $true
if ($d.ShowDialog() -eq 'OK') { $d.FileNames | ForEach-Object { Write-Output $_ } }`)
}

// chooseFolder shows the system's own window for choosing a folder; "" if none was chosen.
func chooseFolder(prompt string) string {
	found := powershell(`Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '` + strings.ReplaceAll(prompt, "'", "") + `'
if ($d.ShowDialog() -eq 'OK') { Write-Output $d.SelectedPath }`)
	if len(found) > 0 {
		return found[0]
	}
	return ""
}
