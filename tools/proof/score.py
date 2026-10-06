"""score.py <out dir> [-v name]: compares what the program read with what was written, song by song."""
import json, sys, glob, os
import numpy as np
S = os.environ.get('PROOF', '.')
N = 'C C# D D# E F F# G G# A A# B'.split()
name = lambda m: f"{N[m%12]}{m//12-1}"
TOL = 0.09
def score(truth, proj, verbose=False):
    notes, ev = truth['notes'], proj['events']
    ts = np.array([n['start'] for n in notes]); es = np.array([e['start'] for e in ev]) if ev else np.array([1e9])
    best = max(np.arange(-0.15, 0.15, 0.005), key=lambda s: sum(np.min(np.abs(es - (t + s))) < 0.05 for t in ts))
    used = set(); right = octave = wrong = missed = 0; lines = []
    for n in notes:
        t = n['start'] + best
        cand = [(abs(e['start'] - t), i) for i, e in enumerate(ev) if abs(e['start'] - t) <= TOL and i not in used]
        if not cand:
            missed += 1; lines.append(f"  missed {name(n['midi'])} at {t:.2f} ({n['end']-n['start']:.2f}s)"); continue
        d, i = min(cand); used.add(i); e = ev[i]
        if e['midi'] == n['midi']: right += 1
        elif (e['midi'] - n['midi']) % 12 == 0: octave += 1; lines.append(f"  octave {name(n['midi'])} written, {name(e['midi'])} read at {t:.2f}")
        else: wrong += 1; lines.append(f"  wrong  {name(n['midi'])} written, {name(e['midi'])} read at {t:.2f}")
    extra = 0
    for i, e in enumerate(ev):
        if i not in used:
            extra += 1; lines.append(f"  extra  {name(e['midi'])} at {e['start']:.2f}-{e['end']:.2f} conf {e.get('confidence',0):.2f}")
    # the octave the whole part sits in: a file written an octave down is no fault of the reader
    r = proj.get('rhythm') or {}
    beats = np.array(r.get('beats') or [0, 1]); tempo = 60 / np.median(np.diff(beats))
    ratio = tempo / truth['tempo'] if truth['tempo'] else 0
    per = r.get('perBar', 4); down = r.get('downbeat', 0)
    bars = beats[down::per]
    td = np.array(truth['downbeats'])
    on_one = np.mean([np.min(np.abs(bars - d)) < 0.08 for d in td]) if len(td) and len(bars) else 0
    tb = np.array(truth['beats'])
    on_beat = np.mean([np.min(np.abs(beats - b)) < 0.06 for b in tb]) if len(tb) else 0
    if verbose: print('\n'.join(lines))
    return dict(written=len(notes), read=len(ev), right=right, octave=octave, wrong=wrong, missed=missed, extra=extra, ratio=ratio, on_beat=on_beat, on_one=on_one)
if __name__ == '__main__':
    out = sys.argv[1]; verbose = sys.argv[3] if len(sys.argv) > 3 else None
    for kind in ['stem', 'full']:
        tot = dict(written=0, right=0, octave=0, wrong=0, missed=0, extra=0)
        print(f"== {kind}: " + ("the reader on the true bass" if kind == 'stem' else "the whole program, from the mix"))
        print(f"{'song':12} written  read right   %  oct wrong miss extra | tempo× beats bars")
        for t in sorted(glob.glob(S + '/set/*.truth.json')):
            n = os.path.basename(t)[:-11]; p = f"{out}/{kind}/{n}.cbass.json"
            if not os.path.exists(p): print(f"{n:12} (not there)"); continue
            s = score(json.load(open(t)), json.load(open(p)), verbose == n)
            for k in tot: tot[k] += s[k]
            print(f"{n:12} {s['written']:7d} {s['read']:5d} {s['right']:5d} {100*s['right']/s['written']:3.0f} {s['octave']:4d} {s['wrong']:5d} {s['missed']:4d} {s['extra']:5d} | {s['ratio']:5.2f} {100*s['on_beat']:4.0f}% {100*s['on_one']:3.0f}%")
        if tot['written']: print(f"{'ALL':12} {tot['written']:7d} {'':5} {tot['right']:5d} {100*tot['right']/tot['written']:3.0f} {tot['octave']:4d} {tot['wrong']:5d} {tot['missed']:4d} {tot['extra']:5d}")
