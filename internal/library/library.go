// Package library keeps the recordings already analysed, so that each is worked out once:
// a folder per recording with its project and the two tracks to play.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/project"
)

// Entry is what the list shows about one recording.
type Entry struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Duration float64   `json:"duration"`
	Tempo    float64   `json:"tempo"`
	Notes    int       `json:"notes"`
	Added    time.Time `json:"added"`
	Key      string    `json:"key,omitempty"`
	BuiltIn  bool      `json:"builtIn,omitempty"`
}

// Library is a folder of analysed recordings.
type Library struct{ Dir string }

// Open returns the library of this user, creating its folder if need be. CBASS_LIBRARY
// names another folder.
func Open() (*Library, error) {
	dir := os.Getenv("CBASS_LIBRARY")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "C_bass", "library")
	}
	return &Library{Dir: dir}, os.MkdirAll(dir, 0o755)
}

// ID names a recording by its content: the same file is recognised whatever it is called.
func ID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// Has says whether that recording is in the library, complete.
func (l *Library) Has(id string) bool {
	for _, name := range []string{"info.json", "project.json", "bass.wav", "backing.wav"} {
		if info, err := os.Stat(filepath.Join(l.Dir, id, name)); err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

// List returns the recordings, the most recent first.
func (l *Library) List() []Entry {
	folders, _ := os.ReadDir(l.Dir)
	var entries []Entry
	for _, folder := range folders {
		if !folder.IsDir() || !l.Has(folder.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(l.Dir, folder.Name(), "info.json"))
		if err != nil {
			continue
		}
		var entry Entry
		if json.Unmarshal(data, &entry) != nil {
			continue
		}
		entry.ID = folder.Name()
		entries = append(entries, entry)
	}
	// one's own recordings first, the most recent on top; the pieces that came with the program after
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].BuiltIn != entries[j].BuiltIn {
			return !entries[i].BuiltIn
		}
		if entries[i].BuiltIn {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Added.After(entries[j].Added)
	})
	return entries
}

// Save stores an analysed recording. The folder is written under another name and moved into
// place at the end, so a half-written one is never taken for a whole one.
func (l *Library) Save(id string, result *project.Result) error {
	if result.Bass == nil || result.Backing == nil {
		return fmt.Errorf("the recording was not separated: there is nothing to play")
	}
	temporary := filepath.Join(l.Dir, id+".part")
	if err := os.RemoveAll(temporary); err != nil {
		return err
	}
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return err
	}
	if err := audio.WriteWAV(filepath.Join(temporary, "bass.wav"), result.Bass); err != nil {
		return err
	}
	if err := audio.WriteWAV(filepath.Join(temporary, "backing.wav"), result.Backing); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(temporary, "project.json"), result.Project); err != nil {
		return err
	}
	entry := Entry{ID: id, Title: result.Project.Title, Duration: result.Project.Duration, Notes: len(result.Project.Events), Added: time.Now(), Key: result.Project.Key, BuiltIn: result.Project.BuiltIn}
	if result.Project.Rhythm != nil {
		entry.Tempo = result.Project.Rhythm.Tempo()
	}
	if err := writeJSON(filepath.Join(temporary, "info.json"), entry); err != nil {
		return err
	}
	final := filepath.Join(l.Dir, id)
	if err := os.RemoveAll(final); err != nil {
		return err
	}
	return os.Rename(temporary, final)
}

// SaveSound replaces the two tracks of a recording and leaves its project as it is. Each is
// written beside the old one and moved over it, so a track is never left half written.
func (l *Library) SaveSound(id string, bass, backing *audio.Buffer) error {
	for name, track := range map[string]*audio.Buffer{"bass.wav": bass, "backing.wav": backing} {
		path := filepath.Join(l.Dir, id, name)
		if err := audio.WriteWAV(path+".part", track); err != nil {
			return err
		}
		if err := os.Rename(path+".part", path); err != nil {
			return err
		}
	}
	return nil
}

// SaveProject stores the project again, after a change of fingering, of bars or of notes.
func (l *Library) SaveProject(id string, p *project.Project) error {
	if err := writeJSON(filepath.Join(l.Dir, id, "project.json"), p); err != nil {
		return err
	}
	var entry Entry
	path := filepath.Join(l.Dir, id, "info.json")
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &entry) == nil && entry.Notes != len(p.Events) {
		entry.Notes = len(p.Events)
		return writeJSON(path, entry)
	}
	return nil
}

// Load reads a recording back: its project, the bass alone and the rest without the bass.
func (l *Library) Load(id string) (*project.Result, error) {
	data, err := os.ReadFile(filepath.Join(l.Dir, id, "project.json"))
	if err != nil {
		return nil, err
	}
	result := &project.Result{Project: &project.Project{}}
	if err := json.Unmarshal(data, result.Project); err != nil {
		return nil, err
	}
	if result.Bass, err = audio.Decode(filepath.Join(l.Dir, id, "bass.wav")); err != nil {
		return nil, err
	}
	if result.Backing, err = audio.Decode(filepath.Join(l.Dir, id, "backing.wav")); err != nil {
		return nil, err
	}
	return result, nil
}

// Remove deletes a recording from the library. The original file is not touched.
func (l *Library) Remove(id string) error {
	if id == "" || filepath.Base(id) != id {
		return fmt.Errorf("not a recording of the library: %q", id)
	}
	return os.RemoveAll(filepath.Join(l.Dir, id))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", " ")
	if err != nil {
		return err
	}
	temporary := path + ".part"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
