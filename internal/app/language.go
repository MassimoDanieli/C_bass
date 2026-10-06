package app

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// systemLanguage is the two-letter code of the language the computer is set to, as far as it
// can be told: "it", "en", and so on; "" when it cannot.
func systemLanguage() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); len(v) >= 2 && v != "C" && !strings.HasPrefix(v, "C.") && !strings.HasPrefix(v, "POSIX") {
			return strings.ToLower(v[:2])
		}
	}
	switch runtime.GOOS {
	case "darwin":
		// a program started from the Finder has no LANG: ask the system
		if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.Trim(strings.TrimSpace(line), `",`)
				if len(line) >= 2 && line != "(" && line != ")" {
					return strings.ToLower(line[:2])
				}
			}
		}
	case "windows":
		if out, err := exec.Command("powershell", "-NoProfile", "-Command", "(Get-Culture).TwoLetterISOLanguageName").Output(); err == nil {
			if v := strings.TrimSpace(string(out)); len(v) >= 2 {
				return strings.ToLower(v[:2])
			}
		}
	}
	return ""
}
