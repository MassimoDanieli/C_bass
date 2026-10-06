package player

import (
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/MassimoDanieli/c_bass/internal/audio"
)

var (
	deviceOnce sync.Once
	device     *oto.Context
	deviceErr  error
)

// deviceBuffer is how much sound the device is asked to hold ahead.
const (
	deviceBuffer = 40 * time.Millisecond
	deviceFrames = Rate * int64(deviceBuffer) / int64(time.Second)
)

// openDevice starts the sound output, once for the whole program.
func openDevice() (*oto.Context, error) {
	deviceOnce.Do(func() {
		if os.Getenv("CBASS_NO_AUDIO") != "" {
			deviceErr = errors.New("sound output turned off")
			return
		}
		context, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate: Rate, ChannelCount: 2, Format: oto.FormatFloat32LE, BufferSize: deviceBuffer,
		})
		if err != nil {
			deviceErr = err
			return
		}
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			deviceErr = errors.New("the sound output did not start")
			return
		}
		if err := context.Err(); err != nil {
			deviceErr = err
			return
		}
		device = context
	})
	return device, deviceErr
}

// Player is an Engine connected to the sound output, with a steady reading of where it is.
type Player struct {
	*Engine
	output *oto.Player
	// Silent is set when there is no sound output: the recording then runs against the clock
	// alone, so that the rest of the program still works. Err says why.
	Silent bool
	Err    error
	stop   chan struct{}

	mu      sync.Mutex
	shown   float64
	last    time.Time
	holding time.Time
}

// New starts a player for two tracks, paused at the beginning.
func New(bass, backing *audio.Buffer) *Player {
	p := &Player{Engine: NewEngine(bass, backing), last: time.Now(), stop: make(chan struct{})}
	context, err := openDevice()
	if err != nil {
		p.Silent, p.Err = true, err
		go p.runSilent()
		return p
	}
	p.output = context.NewPlayer(p.Engine)
	p.output.SetBufferSize(8 * 4096) // 93 ms: short enough for the controls to feel immediate
	p.output.Play()
	return p
}

// runSilent reads the output at the pace a sound device would.
func (p *Player) runSilent() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	began := time.Now()
	var read int64
	buffer := make([]byte, 8*Rate/10)
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			due := int64(time.Since(began).Seconds()*Rate) - read
			if due <= 0 {
				continue
			}
			due = min(due, int64(len(buffer)/8))
			p.Engine.Read(buffer[:due*8])
			read += due
		}
	}
}

// Close stops the sound.
func (p *Player) Close() {
	close(p.stop)
	if p.output != nil {
		p.output.Pause()
		p.output.Close()
	}
}

// heard is the output frame reaching the ears now.
func (p *Player) heard() int64 {
	consumed := p.Engine.Consumed()
	if p.output != nil {
		// what is waiting in the player, and what the device itself holds
		consumed -= int64(p.output.BufferedSize()/8) + deviceFrames
	}
	return consumed
}

// Seek moves to a time in seconds; the position shown follows at once, without waiting for
// the sound already on its way out.
func (p *Player) Seek(seconds float64) {
	p.Engine.Seek(seconds)
	p.mu.Lock()
	p.shown = p.Engine.Target()
	p.holding = time.Now().Add(250 * time.Millisecond)
	p.mu.Unlock()
}

// Position is the moment of the recording being heard, in seconds, moving smoothly: the
// sound device reports its progress in steps, too coarse to scroll a page by.
func (p *Player) Position() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(p.last).Seconds()
	p.last = now
	playing, speed := p.Engine.Playing(), p.Engine.Speed()
	if playing {
		p.shown += elapsed * speed
	}
	if now.After(p.holding) {
		target := p.Engine.At(p.heard())
		if gap := target - p.shown; !playing || math.Abs(gap) > 0.2 {
			p.shown = target
		} else {
			p.shown += gap * math.Min(1, elapsed*6)
		}
	}
	p.shown = math.Max(0, math.Min(p.Duration(), p.shown))
	return p.shown
}
