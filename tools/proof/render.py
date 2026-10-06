# render.py <midi> <name> <start> <length>: renders an excerpt of a MIDI song with a sound font:
# the full mix, the bass alone, and the bass notes as written (truth.json).
import os, sys, json, warnings, numpy as np, scipy.io.wavfile as w, mido, tinysoundfont, pretty_midi
warnings.filterwarnings('ignore')
src, name, start, length = sys.argv[1], sys.argv[2], float(sys.argv[3]), float(sys.argv[4])
SF = os.environ.get('SOUNDFONT', 'dl/GeneralUser-GS/GeneralUser-GS.sf2')
SR = 44100
pm = pretty_midi.PrettyMIDI(src)
bass = [i for i in pm.instruments if not i.is_drum and 32 <= i.program <= 39]
# every bass instrument is in the bass stem, so every one is in the truth

def render(only_bass):
    mid = mido.MidiFile(src)
    # which channels carry the bass: those whose program is a bass at the time of their notes
    synth = tinysoundfont.Synth(samplerate=SR); sf = synth.sfload(SF)
    for ch in range(16): synth.program_select(ch, sf, 128 if ch == 9 else 0, 0, ch == 9)
    program = {ch: 0 for ch in range(16)}
    out = []; t = 0.0; done = 0
    def advance(to):
        nonlocal done
        n = int(to * SR) - done
        if n > 0:
            out.append(np.frombuffer(synth.generate(n), dtype=np.float32).copy()); done += n
    for msg in mid:  # merged, with times in seconds
        t += msg.time
        if t > start + length + 2: break
        advance(t)
        if msg.is_meta: continue
        if msg.type == 'program_change':
            program[msg.channel] = msg.program
            if msg.channel != 9: synth.program_select(msg.channel, sf, 0, msg.program, False)
        elif msg.type == 'note_on' and msg.velocity > 0:
            is_bass = msg.channel != 9 and 32 <= program[msg.channel] <= 39
            if (not only_bass) or is_bass: synth.noteon(msg.channel, msg.note, msg.velocity)
        elif msg.type in ('note_off', 'note_on'):
            synth.noteoff(msg.channel, msg.note)
        elif msg.type == 'control_change' and msg.control in (7, 10, 11, 64):
            synth.control_change(msg.channel, msg.control, msg.value)
        elif msg.type == 'pitchwheel':
            synth.pitchbend(msg.channel, msg.pitch + 8192)
    advance(start + length + 2)
    x = np.concatenate(out).reshape(-1, 2)
    return x[int(start*SR):int((start+length)*SR)]

mix, solo = render(False), render(True)
peak = max(abs(mix).max(), 1e-9)
gain = 0.89 / peak
w.write(name + '.wav', SR, (mix * gain * 32767).astype(np.int16))
w.write(name + '.bass.wav', SR, (solo * gain * 32767).astype(np.int16))
notes = [dict(start=n.start - start, end=n.end - start, midi=int(n.pitch)) for i in bass for n in i.notes if start <= n.start < start + length]
notes.sort(key=lambda n: n['start'])
# two bass instruments on the same note at once count once
kept = []
for n in notes:
    if kept and abs(kept[-1]['start'] - n['start']) < 0.03 and (kept[-1]['midi'] - n['midi']) % 12 == 0: continue
    kept.append(n)
notes = kept
beats = [float(b - start) for b in pm.get_beats() if start <= b < start + length]
downs = [float(b - start) for b in pm.get_downbeats() if start <= b < start + length]
tempo = float(np.median(60 / np.diff(pm.get_beats()))) if len(pm.get_beats()) > 2 else 0
json.dump(dict(notes=notes, beats=beats, downbeats=downs, tempo=tempo, source=src.split('/')[-1], start=start), open(name + '.truth.json', 'w'))
rms = lambda x: 20*np.log10(np.sqrt((x**2).mean()) + 1e-9)
print(name, 'notes', len(notes), 'tempo %.0f' % tempo, 'mix rms %.1f bass rms %.1f' % (rms(mix*gain), rms(solo*gain)), 'lowest', min(n['midi'] for n in notes) if notes else None)
