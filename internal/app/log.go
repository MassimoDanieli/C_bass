package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// The program keeps a short diary beside its settings: what it was asked to do, how long each
// step took, and what went wrong, with where in the program it happened. It stays on the
// computer; it is there to be read, or sent along, when something does not work.

const issues = "https://github.com/MassimoDanieli/C_bass/issues/new"

var diary struct {
	sync.Mutex
	file *os.File
}

func diaryPath() string {
	path := settingsPath()
	if path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), "log.txt")
}

// openDiary starts the diary for this run. One that has grown long is started again.
func openDiary(version string) {
	path := diaryPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > 256<<10 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return
	}
	diary.file = file
	note("---- C_bass %s on %s/%s, %d processors", version, runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

// note writes a line in the diary.
func note(format string, args ...any) {
	diary.Lock()
	defer diary.Unlock()
	if diary.file == nil {
		return
	}
	fmt.Fprintf(diary.file, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// crashed is deferred where the program could fall over: it writes down where it did, and
// lets it fall.
func crashed(where string) {
	if r := recover(); r != nil {
		note("CRASH in %s: %v\n%s", where, r, debug.Stack())
		panic(r)
	}
}
