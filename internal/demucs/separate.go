package demucs

import (
	"errors"
	"fmt"
	"math"
	"runtime"

	"github.com/MassimoDanieli/c_bass/internal/audio"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	// SampleRate is the only rate the model was trained on.
	SampleRate = 44100
	// segment is how much audio the network takes at once: 7.8 seconds.
	segment = 343980
	sources = 4
)

// Sources are the stems, in the order the model returns them.
var Sources = [sources]string{"drums", "bass", "other", "vocals"}

// Stems is a separated recording: the "bass", and the "rest", which is the drums, the voice
// and everything else together. The network gives four parts; the three that are not the
// bass are added up as they come, because that is all they are wanted for and a long
// recording kept in four parts takes twice the memory.
type Stems map[string]*audio.Buffer

// Options for a Separator.
type Options struct {
	// Library is the path of the ONNX Runtime shared library.
	Library string
	// Model is the path of htdemucs.onnx.
	Model string
	// Threads to use; 0 means all the cores.
	Threads int
	// CoreML asks ONNX Runtime to use Apple's CoreML where it can (macOS only).
	CoreML bool
}

// ErrStopped is what Separate returns when it was asked to stop.
var ErrStopped = errors.New("stopped")

// Separator holds a loaded model.
type Separator struct {
	// Stop, if set, is asked before every pass whether to give up: a separation takes a
	// minute or more, and whoever started it may think better of it.
	Stop    func() bool
	session *ort.DynamicAdvancedSession
	inputs  []string
	outputs []string
}

var environmentReady bool

// Open loads ONNX Runtime and the model.
func Open(options Options) (*Separator, error) {
	if !environmentReady {
		ort.SetSharedLibraryPath(options.Library)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("starting ONNX Runtime (%s): %w", options.Library, err)
		}
		environmentReady = true
	}
	inputs, outputs, err := ort.GetInputOutputInfo(options.Model)
	if err != nil {
		return nil, fmt.Errorf("reading the model (%s): %w", options.Model, err)
	}
	if len(inputs) != 2 || len(outputs) != 2 {
		return nil, fmt.Errorf("the model has %d inputs and %d outputs, expected 2 and 2", len(inputs), len(outputs))
	}
	settings, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer settings.Destroy()
	threads := options.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if err := settings.SetIntraOpNumThreads(threads); err != nil {
		return nil, err
	}
	// The network needs gigabytes of scratch memory for each window. Without the arena it is
	// given back after every pass instead of being kept, and grown, for the whole recording.
	if err := settings.SetCpuMemArena(false); err != nil {
		return nil, err
	}
	// The runtime's rewriting of the network before running it buys no speed with this model
	// and costs memory: measured on the same recording, about 2.4 GB at the peak without it
	// against 3 to 4.3 GB with it, in the same time, the result the same to the last bit but one.
	if err := settings.SetGraphOptimizationLevel(ort.GraphOptimizationLevelDisableAll); err != nil {
		return nil, err
	}
	if options.CoreML {
		if err := settings.AppendExecutionProviderCoreMLV2(map[string]string{"MLComputeUnits": "ALL"}); err != nil {
			return nil, fmt.Errorf("CoreML is not available: %w", err)
		}
	}
	s := &Separator{}
	for _, info := range inputs {
		s.inputs = append(s.inputs, info.Name)
	}
	for _, info := range outputs {
		s.outputs = append(s.outputs, info.Name)
	}
	s.session, err = ort.NewDynamicAdvancedSession(options.Model, s.inputs, s.outputs, settings)
	if err != nil {
		return nil, fmt.Errorf("loading the model: %w", err)
	}
	return s, nil
}

// Close frees the model.
func (s *Separator) Close() error { return s.session.Destroy() }

// separator is the working memory for one run.
type separator struct {
	fft     *transform
	window  []float64
	re, im  []float64
	overlap []float64
	weight  []float64
}

// Separate splits a 44.1 kHz stereo recording into its four stems. Progress is called with the
// number of passes done and the total.
func (s *Separator) Separate(mix *audio.Buffer, progress func(done, total int)) (Stems, error) {
	if mix.SampleRate != SampleRate || len(mix.Channels) != 2 {
		return nil, fmt.Errorf("the model needs 44.1 kHz stereo, got %d Hz with %d channels", mix.SampleRate, len(mix.Channels))
	}
	length := mix.Len()
	work := &separator{
		fft: newTransform(fftSize), window: make([]float64, fftSize),
		re: make([]float64, fftSize), im: make([]float64, fftSize),
		overlap: make([]float64, segment+8*hop),
	}
	for i := range work.window {
		work.window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/fftSize))
	}
	frames := (segment + hop - 1) / hop

	out := make([][]float32, 4) // the bass, left and right; then the rest
	for i := range out {
		out[i] = make([]float32, length)
	}
	sumWeight := make([]float32, length)
	// Each pass overlaps the next by a quarter and fades in and out, so the joins cannot be heard.
	stride := segment * 3 / 4
	weight := make([]float32, segment)
	for i := range weight {
		if i <= segment/2 {
			weight[i] = float32(i + 1)
		} else {
			weight[i] = float32(segment - i)
		}
	}
	peak := weight[segment/2]
	for i := range weight {
		weight[i] /= peak
	}

	chunkMix := make([]float32, 2*segment)
	spectrum := make([]float32, 4*bins*frames)
	chunkOut := make([]float32, segment)
	total := (length + stride - 1) / stride
	if progress != nil {
		progress(0, total)
	}
	for pass, offset := 0, 0; offset < length; pass, offset = pass+1, offset+stride {
		if s.Stop != nil && s.Stop() {
			return nil, ErrStopped
		}
		chunk := min(segment, length-offset)
		// The stretch is centred in the network's window, with real audio around it where there is some.
		start := offset - (segment-chunk)/2
		for c := 0; c < 2; c++ {
			row := chunkMix[c*segment : (c+1)*segment]
			for i := range row {
				if p := start + i; p >= 0 && p < length {
					row[i] = mix.Channels[c][p]
				} else {
					row[i] = 0
				}
			}
			work.spectrogram(row, spectrum[(2*c)*bins*frames:(2*c+1)*bins*frames], spectrum[(2*c+1)*bins*frames:(2*c+2)*bins*frames], frames)
		}
		masks, waves, release, err := s.run(chunkMix, spectrum, frames)
		if err != nil {
			return nil, err
		}
		trim := (segment - chunk) / 2
		for source := 0; source < sources; source++ {
			for c := 0; c < 2; c++ {
				copy(chunkOut, waves[(source*2+c)*segment:(source*2+c+1)*segment])
				plane := bins * frames
				base := (source*4 + 2*c) * plane
				work.inverse(masks[base:base+plane], masks[base+plane:base+2*plane], frames, segment, chunkOut)
				target := out[2+c]
				if Sources[source] == "bass" {
					target = out[c]
				}
				for i := 0; i < chunk; i++ {
					target[offset+i] += weight[i] * chunkOut[trim+i]
				}
			}
		}
		release()
		for i := 0; i < chunk; i++ {
			sumWeight[offset+i] += weight[i]
		}
		if progress != nil {
			progress(pass+1, total)
		}
	}
	for _, row := range out {
		for i := range row {
			row[i] /= sumWeight[i]
		}
	}
	return Stems{
		"bass": &audio.Buffer{SampleRate: SampleRate, Channels: out[:2]},
		"rest": &audio.Buffer{SampleRate: SampleRate, Channels: out[2:]},
	}, nil
}

// run feeds one window to the network and returns its two answers: the spectrogram of each
// source as [source][real/imag per channel][bin][frame], and each source's waveform. The
// slices belong to ONNX Runtime and are only valid until release is called.
func (s *Separator) run(mix, spectrum []float32, frames int) (masks, waves []float32, release func(), err error) {
	mixTensor, err := ort.NewTensor(ort.NewShape(1, 2, segment), mix)
	if err != nil {
		return nil, nil, nil, err
	}
	defer mixTensor.Destroy()
	spectrumTensor, err := ort.NewTensor(ort.NewShape(1, 4, bins, int64(frames)), spectrum)
	if err != nil {
		return nil, nil, nil, err
	}
	defer spectrumTensor.Destroy()
	results := []ort.Value{nil, nil}
	if err := s.session.Run([]ort.Value{mixTensor, spectrumTensor}, results); err != nil {
		return nil, nil, nil, fmt.Errorf("running the model: %w", err)
	}
	release = func() {
		results[0].Destroy()
		results[1].Destroy()
	}
	first, ok1 := results[0].(*ort.Tensor[float32])
	second, ok2 := results[1].(*ort.Tensor[float32])
	if !ok1 || !ok2 {
		release()
		return nil, nil, nil, fmt.Errorf("the model returned something other than float tensors")
	}
	masks, waves = first.GetData(), second.GetData()
	if len(masks) != sources*4*bins*frames || len(waves) != sources*2*segment {
		release()
		return nil, nil, nil, fmt.Errorf("the model returned %d and %d values, expected %d and %d", len(masks), len(waves), sources*4*bins*frames, sources*2*segment)
	}
	return masks, waves, release, nil
}
