// Package audio reads recordings into memory and writes them back out.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"
)

// Buffer is a recording in memory: one slice of samples per channel, all the same length.
type Buffer struct {
	SampleRate int
	Channels   [][]float32
}

// Len is the number of sample frames.
func (b *Buffer) Len() int {
	if len(b.Channels) == 0 {
		return 0
	}
	return len(b.Channels[0])
}

// Duration is the length in seconds.
func (b *Buffer) Duration() float64 { return float64(b.Len()) / float64(b.SampleRate) }

// Mono is the average of the channels.
func (b *Buffer) Mono() []float32 {
	out := make([]float32, b.Len())
	scale := 1 / float32(len(b.Channels))
	for _, channel := range b.Channels {
		for i, v := range channel {
			out[i] += v * scale
		}
	}
	return out
}

// Stereo returns a two-channel view: a mono recording is doubled, extra channels are dropped.
func (b *Buffer) Stereo() *Buffer {
	switch len(b.Channels) {
	case 2:
		return b
	case 1:
		return &Buffer{SampleRate: b.SampleRate, Channels: [][]float32{b.Channels[0], append([]float32(nil), b.Channels[0]...)}}
	default:
		return &Buffer{SampleRate: b.SampleRate, Channels: b.Channels[:2]}
	}
}

// Decode reads a WAV or MP3 file. The format is taken from the content, not from the name.
func Decode(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeBytes(data, path)
}

// DecodeBytes reads a WAV or MP3 file already in memory; the name is only used as a hint and
// in error messages.
func DecodeBytes(data []byte, path string) (*Buffer, error) {
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return ReadWAV(data)
	}
	if len(data) >= 4 && string(data[0:4]) == "fLaC" {
		return readFLAC(data)
	}
	// The other kinds come before MP3, which has no mark of its own and is guessed at.
	if kind := otherKind(data, path); kind != "" {
		return readWithHelp(data, kind, path)
	}
	if looksLikeMP3(data) || strings.EqualFold(filepath.Ext(path), ".mp3") {
		return readMP3(data)
	}
	return nil, fmt.Errorf("%s: only WAV, MP3, FLAC and M4A files can be read", filepath.Base(path))
}

// Kinds are the endings of the files Decode may be able to read.
var Kinds = []string{".mp3", ".wav", ".wave", ".flac", ".m4a", ".aac", ".mp4", ".ogg", ".opus", ".aif", ".aiff"}

// Readable says whether a file has one of those endings.
func Readable(name string) bool {
	ending := strings.ToLower(filepath.Ext(name))
	for _, kind := range Kinds {
		if ending == kind {
			return true
		}
	}
	return false
}

func readFLAC(data []byte) (*Buffer, error) {
	stream, err := flac.New(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("flac: %w", err)
	}
	channels := int(stream.Info.NChannels)
	scale := float32(1) / float32(int64(1)<<(stream.Info.BitsPerSample-1))
	out := &Buffer{SampleRate: int(stream.Info.SampleRate), Channels: make([][]float32, channels)}
	for {
		frame, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("flac: %w", err)
		}
		for c, subframe := range frame.Subframes {
			for _, sample := range subframe.Samples {
				out.Channels[c] = append(out.Channels[c], float32(sample)*scale)
			}
		}
	}
	if out.Len() == 0 {
		return nil, errors.New("flac: no audio in the file")
	}
	return out, nil
}

// otherKind recognises the kinds of file this program has no reader of its own for: AAC in
// its MP4 wrapping (.m4a), Ogg, AIFF. It goes by the content first, then by the name.
func otherKind(data []byte, path string) string {
	switch {
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return ".m4a"
	case len(data) >= 4 && string(data[0:4]) == "OggS":
		return ".ogg"
	case len(data) >= 12 && string(data[0:4]) == "FORM" && (string(data[8:12]) == "AIFF" || string(data[8:12]) == "AIFC"):
		return ".aiff"
	}
	switch ending := strings.ToLower(filepath.Ext(path)); ending {
	case ".m4a", ".aac", ".mp4", ".ogg", ".opus", ".aif", ".aiff":
		return ending
	}
	return ""
}

// readWithHelp has the system turn a file into WAV: afconvert, which every Mac has, or ffmpeg
// where it is installed.
func readWithHelp(data []byte, kind, path string) (*Buffer, error) {
	dir, err := os.MkdirTemp("", "cbass")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in"+kind), filepath.Join(dir, "out.wav")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}
	var tried []string
	for _, command := range [][]string{
		{"afconvert", "-f", "WAVE", "-d", "LEI16", in, out},
		{"ffmpeg", "-v", "error", "-y", "-i", in, "-vn", "-acodec", "pcm_s16le", out},
	} {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		tried = append(tried, command[0])
		if err := hidden(exec.Command(command[0], command[1:]...)).Run(); err != nil {
			continue
		}
		if converted, err := os.ReadFile(out); err == nil {
			if buffer, err := ReadWAV(converted); err == nil {
				return buffer, nil
			}
		}
	}
	name := filepath.Base(path)
	if len(tried) == 0 {
		return nil, fmt.Errorf("%s: %s files need ffmpeg, which is not installed: convert the file to MP3, WAV or FLAC", name, kind)
	}
	return nil, fmt.Errorf("%s: %s could not read this file", name, strings.Join(tried, " and "))
}

func looksLikeMP3(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	return string(data[0:3]) == "ID3" || (data[0] == 0xff && data[1]&0xe0 == 0xe0)
}

func readMP3(data []byte) (*Buffer, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mp3: %w", err)
	}
	pcm, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("mp3: %w", err)
	}
	frames := len(pcm) / 4 // always 16-bit stereo
	// An MP3 carries silence before and after the music, put there by the encoder and the
	// decoder. Players leave it out, and so does this, or every time would be a little late.
	skip, trim := mp3Padding(data)
	if skip+trim < frames {
		pcm = pcm[skip*4 : (frames-trim)*4]
		frames -= skip + trim
	}
	left := make([]float32, frames)
	right := make([]float32, frames)
	for i := 0; i < frames; i++ {
		left[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*4:]))) / 32768
		right[i] = float32(int16(binary.LittleEndian.Uint16(pcm[i*4+2:]))) / 32768
	}
	return &Buffer{SampleRate: decoder.SampleRate(), Channels: [][]float32{left, right}}, nil
}

// ReadWAV decodes PCM (8, 16, 24 or 32 bit) and 32-bit float WAV data.
func ReadWAV(data []byte) (*Buffer, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("wav: not a RIFF/WAVE file")
	}
	var format, channels, bits, rate int
	offset := 12
	for offset+8 <= len(data) {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4:]))
		body := offset + 8
		if body+size > len(data) {
			size = len(data) - body
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("wav: short format chunk")
			}
			format = int(binary.LittleEndian.Uint16(data[body:]))
			channels = int(binary.LittleEndian.Uint16(data[body+2:]))
			rate = int(binary.LittleEndian.Uint32(data[body+4:]))
			bits = int(binary.LittleEndian.Uint16(data[body+14:]))
			if format == 0xfffe && size >= 26 { // extensible: the real format is in the sub-format
				format = int(binary.LittleEndian.Uint16(data[body+24:]))
			}
		case "data":
			if channels == 0 || bits == 0 {
				return nil, errors.New("wav: data before format")
			}
			width := bits / 8
			frames := size / (width * channels)
			out := make([][]float32, channels)
			for c := range out {
				out[c] = make([]float32, frames)
			}
			for i := 0; i < frames; i++ {
				for c := 0; c < channels; c++ {
					p := body + (i*channels+c)*width
					var v float32
					switch {
					case format == 3 && bits == 32:
						v = math.Float32frombits(binary.LittleEndian.Uint32(data[p:]))
					case format == 1 && bits == 8:
						v = (float32(data[p]) - 128) / 128
					case format == 1 && bits == 16:
						v = float32(int16(binary.LittleEndian.Uint16(data[p:]))) / 32768
					case format == 1 && bits == 24:
						v = float32(int32(uint32(data[p])<<8|uint32(data[p+1])<<16|uint32(data[p+2])<<24)>>8) / 8388608
					case format == 1 && bits == 32:
						v = float32(int32(binary.LittleEndian.Uint32(data[p:]))) / 2147483648
					default:
						return nil, fmt.Errorf("wav: format %d with %d bits is not supported", format, bits)
					}
					out[c][i] = v
				}
			}
			return &Buffer{SampleRate: rate, Channels: out}, nil
		}
		offset = body + size + size%2
	}
	return nil, errors.New("wav: no audio data")
}

// WriteWAV saves a buffer as 16-bit PCM.
func WriteWAV(path string, b *Buffer) error {
	frames, channels := b.Len(), len(b.Channels)
	size := frames * channels * 2
	out := make([]byte, 44+size)
	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+size))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(b.SampleRate))
	binary.LittleEndian.PutUint32(out[28:], uint32(b.SampleRate*channels*2))
	binary.LittleEndian.PutUint16(out[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(size))
	p := 44
	for i := 0; i < frames; i++ {
		for c := 0; c < channels; c++ {
			v := float64(b.Channels[c][i])
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
			binary.LittleEndian.PutUint16(out[p:], uint16(int16(math.Round(v*32767))))
			p += 2
		}
	}
	return os.WriteFile(path, out, 0o644)
}

// Resample converts a buffer to another sample rate with a windowed-sinc filter.
func Resample(b *Buffer, rate int) *Buffer {
	if b.SampleRate == rate {
		return b
	}
	ratio := float64(rate) / float64(b.SampleRate)
	cutoff := math.Min(1, ratio) * 0.97 // of the source Nyquist
	const half = 24                     // zero crossings on each side
	reach := int(math.Ceil(half / cutoff))
	frames := int(math.Floor(float64(b.Len()) * ratio))
	out := &Buffer{SampleRate: rate, Channels: make([][]float32, len(b.Channels))}
	for c, source := range b.Channels {
		target := make([]float32, frames)
		for i := range target {
			position := float64(i) / ratio
			centre := int(math.Floor(position))
			var sum, weight float64
			for j := centre - reach + 1; j <= centre+reach; j++ {
				if j < 0 || j >= len(source) {
					continue
				}
				x := (position - float64(j)) * cutoff
				w := sinc(x) * hann(x/half)
				sum += float64(source[j]) * w
				weight += w
			}
			if weight != 0 {
				target[i] = float32(sum / weight)
			}
		}
		out.Channels[c] = target
	}
	return out
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func hann(x float64) float64 {
	if x <= -1 || x >= 1 {
		return 0
	}
	return 0.5 + 0.5*math.Cos(math.Pi*x)
}

// mp3Padding works out how many samples to leave out at the start and at the end of a decoded
// MP3. The encoder says how much it added in the LAME part of the first frame; the decoder
// itself adds 529 samples, and that first frame, which holds no music, decodes as silence.
func mp3Padding(data []byte) (skip, trim int) {
	const decoderDelay = 529
	position := 0
	if len(data) > 10 && string(data[0:3]) == "ID3" {
		position = 10 + int(data[6]&0x7f)<<21 | int(data[7]&0x7f)<<14 | int(data[8]&0x7f)<<7 | int(data[9]&0x7f)
		if data[5]&0x10 != 0 {
			position += 10
		}
	}
	for ; position+4 < len(data); position++ {
		if data[position] == 0xff && data[position+1]&0xe0 == 0xe0 {
			break
		}
	}
	if position+200 > len(data) {
		return decoderDelay + 576, 0
	}
	mpeg1 := (data[position+1]>>3)&3 == 3
	mono := (data[position+3]>>6)&3 == 3
	samples, side := 576, 17
	switch {
	case mpeg1 && !mono:
		samples, side = 1152, 32
	case mpeg1:
		samples, side = 1152, 17
	case mono:
		side = 9
	}
	tag := position + 4 + side
	if id := string(data[tag : tag+4]); id != "Xing" && id != "Info" {
		return decoderDelay + 576, 0
	}
	flags := data[tag+7]
	offset := tag + 8
	for bit, size := range []int{4, 4, 100, 4} { // frame count, byte count, seek table, quality
		if flags&(1<<bit) != 0 {
			offset += size
		}
	}
	lame := offset + 21 // past the encoder name and the settings that precede the two counts
	if lame+3 > len(data) {
		return samples + decoderDelay + 576, 0
	}
	delay := int(data[lame])<<4 | int(data[lame+1])>>4
	padding := int(data[lame+1]&0x0f)<<8 | int(data[lame+2])
	return samples + delay + decoderDelay, max(0, padding-decoderDelay)
}
