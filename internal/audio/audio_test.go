package audio

import (
	"math"
	"path/filepath"
	"testing"
)

func sine(rate int, seconds, frequency float64) []float32 {
	out := make([]float32, int(float64(rate)*seconds))
	for i := range out {
		out[i] = float32(0.5 * math.Sin(2*math.Pi*frequency*float64(i)/float64(rate)))
	}
	return out
}

func TestWAVRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.wav")
	in := &Buffer{SampleRate: 44100, Channels: [][]float32{sine(44100, 0.5, 110), sine(44100, 0.5, 220)}}
	if err := WriteWAV(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Decode(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.SampleRate != 44100 || len(out.Channels) != 2 || out.Len() != in.Len() {
		t.Fatalf("got %d Hz, %d channels, %d frames", out.SampleRate, len(out.Channels), out.Len())
	}
	for c := range in.Channels {
		for i, v := range in.Channels[c] {
			if math.Abs(float64(v-out.Channels[c][i])) > 1.0/32000 {
				t.Fatalf("channel %d sample %d: wrote %f, read %f", c, i, v, out.Channels[c][i])
			}
		}
	}
}

func TestResampleKeepsThePitch(t *testing.T) {
	in := &Buffer{SampleRate: 48000, Channels: [][]float32{sine(48000, 1, 440)}}
	out := Resample(in, 44100)
	if out.SampleRate != 44100 || out.Len() != 44100 {
		t.Fatalf("got %d frames at %d Hz", out.Len(), out.SampleRate)
	}
	want := sine(44100, 1, 440)
	var worst float64
	for i := 2000; i < len(want)-2000; i++ {
		worst = math.Max(worst, math.Abs(float64(want[i]-out.Channels[0][i])))
	}
	if worst > 0.002 {
		t.Fatalf("a 440 Hz tone came back up to %f away from itself", worst)
	}
}

func TestStereoAndMono(t *testing.T) {
	mono := &Buffer{SampleRate: 8000, Channels: [][]float32{{0.2, 0.4}}}
	stereo := mono.Stereo()
	if len(stereo.Channels) != 2 || stereo.Channels[1][1] != 0.4 {
		t.Fatal("a mono recording should be doubled")
	}
	both := &Buffer{SampleRate: 8000, Channels: [][]float32{{1, 0}, {0, 1}}}
	if m := both.Mono(); m[0] != 0.5 || m[1] != 0.5 {
		t.Fatalf("mono mix is %v", m)
	}
}
