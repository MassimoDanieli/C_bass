package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MassimoDanieli/c_bass/internal/export"
	"github.com/MassimoDanieli/c_bass/internal/tab"
)

// downloads is the folder files are saved to: the user's Downloads, whatever it is called.
func downloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	if dir := os.Getenv("CBASS_EXPORTS"); dir != "" {
		return dir
	}
	if runtime.GOOS == "linux" { // it has a name in every language there
		if out, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
			if dir := strings.TrimSpace(string(out)); dir != "" && dir != home {
				return dir
			}
		}
	}
	if dir := filepath.Join(home, "Downloads"); isDir(dir) {
		return dir
	}
	return home
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileName makes a title safe to use as the name of a file.
func fileName(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" {
		name = "C_bass"
	}
	return name
}

// export writes the part to the Downloads folder in one of three forms, and says where.
func (g *Game) export(s *song, kind string) {
	title := g.titleOf(s.project.Title, s.project.Key)
	var data []byte
	ending := ""
	switch kind {
	case "pdf":
		copied := *s.project
		copied.Title = title
		data, ending = export.PDF(&copied, g.noteName), ".pdf"
	case "musicxml":
		copied := *s.project
		copied.Title = title
		data, ending = export.MusicXML(&copied), ".musicxml"
	default:
		data, ending = []byte(tab.Text(title, s.project.Events, s.pulse, s.tuning, 4)), ".tab.txt"
	}
	path := filepath.Join(downloads(), fileName(title)+ending)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		g.say(err.Error())
		return
	}
	g.tell(g.t("Salvato in ", "Saved in ") + filepath.Base(filepath.Dir(path)) + ": " + filepath.Base(path))
	if kind == "pdf" && g.shot == "" {
		show(path)
	}
}

// show opens a file with whatever the system opens it with.
func show(path string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", path).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
}
