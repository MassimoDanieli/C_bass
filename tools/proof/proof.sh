#!/bin/bash
# proof.sh <cbass> [folder]: measures the program against songs whose bass part is known.
#
# Songs written as MIDI are played with a sound font: the whole band for the program to
# listen to, the bass alone, and the bass notes as written. The program is then run twice on
# each: on the true bass (which measures the reader of notes and the finder of bars) and on
# the whole recording (which adds the separation). score.py counts what came out.
#
# Needs python3 with numpy, scipy, mido, pretty_midi and tinysoundfont, and git. The songs
# and the sound font are fetched, not kept here: they are other people's.
set -e
bin=$(realpath "$1"); work=${2:-proof-work}; here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$work/dl" "$work/set" "$work/out/stem" "$work/out/full"; cd "$work"
[ -d dl/MidiTok ] || git clone -q --depth 1 --filter=blob:none --sparse https://github.com/Natooz/MidiTok.git dl/MidiTok
(cd dl/MidiTok && git sparse-checkout set tests/MIDIs_multitrack >/dev/null)
[ -d dl/GeneralUser-GS ] || git clone -q --depth 1 https://github.com/mrbumpy409/GeneralUser-GS.git dl/GeneralUser-GS
songs=dl/MidiTok/tests/MIDIs_multitrack
while IFS='|' read -r file name; do
  [ -f "set/$name.truth.json" ] || python3 "$here/render.py" "$songs/$file" "set/$name" 45 75
done <<'LIST'
Aicha.mid|aicha
All The Small Things.mid|smallthings
Girls Just Want to Have Fun.mid|girls
In Too Deep.mid|toodeep
Les Yeux Revolvers.mid|yeux
Mr. Blue Sky.mid|bluesky
Shut Up.mid|shutup
What a Fool Believes.mid|fool
LIST
for truth in set/*.truth.json; do
  name=$(basename "$truth" .truth.json)
  "$bin" -bass "set/$name.bass.wav" -o out/stem "set/$name.wav" > "out/stem/$name.log" 2>&1
  [ -n "$PROOF_QUICK" ] || "$bin" -stems -o out/full "set/$name.wav" > "out/full/$name.log" 2>&1
done
PROOF=. python3 "$here/score.py" out
