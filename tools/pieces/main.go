// Command pieces writes the recordings that come with the program as files, for Manico, the
// web app C_bass comes from, which ships the same five: for each piece the bass alone and the
// rest alone as WAV, and for all of them a pieces.json with the notes, the chords and the pulse.
//
//	go run ./tools/pieces folder
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	"github.com/MassimoDanieli/c_bass/internal/demo"
)

type note struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Midi  int     `json:"midi"`
}

type chord struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Root    int     `json:"root"`
	Quality string  `json:"quality"`
}

type piece struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Key      string  `json:"key"`
	BPM      float64 `json:"bpm"`
	Beats    int     `json:"beats"`
	Duration float64 `json:"duration"`
	Notes    []note  `json:"notes"`
	Chords   []chord `json:"chords"`
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pieces folder")
		os.Exit(2)
	}
	folder := os.Args[1]
	fail := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "pieces:", err)
			os.Exit(1)
		}
	}
	fail(os.MkdirAll(folder, 0o755))
	var all []piece
	for _, p := range demo.Pieces {
		result := p.Render()
		bass, backing := demo.Buffers(result)
		fail(audio.WriteWAV(filepath.Join(folder, p.ID+"-bass.wav"), bass))
		fail(audio.WriteWAV(filepath.Join(folder, p.ID+"-backing.wav"), backing))
		out := piece{ID: p.ID, Title: p.Title, Key: p.Key, BPM: p.BPM, Beats: p.Beats, Duration: round(result.Project.Duration)}
		for _, e := range result.Project.Events {
			out.Notes = append(out.Notes, note{round(e.Start), round(e.End), e.Midi})
		}
		for _, c := range result.Project.Chords {
			out.Chords = append(out.Chords, chord{round(c.Start), round(c.End), c.Root, c.Quality})
		}
		all = append(all, out)
	}
	data, err := json.Marshal(all)
	fail(err)
	fail(os.WriteFile(filepath.Join(folder, "pieces.json"), data, 0o644))
}
