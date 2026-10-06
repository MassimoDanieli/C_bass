# C_bass

Dal brano alla parte di basso, come programma a sé: niente browser, niente JavaScript. È [Manico](https://github.com/MassimoDanieli/accordi_di_basso) riscritto in Go.

*From a recording to a bass part, as a program of its own: no browser, no JavaScript. It is [Manico](https://github.com/MassimoDanieli/accordi_di_basso) rewritten in Go. English follows the Italian.*

## A che punto è

| | |
|---|---|
| **Motore** (separazione del basso, note, tempo e battute, diteggiatura, tablatura) | fatto, da riga di comando |
| **Interfaccia** (riproduzione con velocità variabile, tablatura che scorre, manico) | da fare |
| **Applicazione** per macOS (Apple Silicon) e Windows, con icona e installazione | da fare |

Il motore dà gli stessi risultati di Manico 7.2.0: sul brano di prova (4:48) le 984 note e i 705 beat coincidono uno per uno.

## Uso

```
cbass analyse -stems brano.mp3
```

Scrive accanto al brano:

- `brano.cbass.json`: note, battute e diteggiatura;
- `brano.tab.txt`: la tablatura, con battute e valori ritmici;
- `brano.no-bass.wav` e `brano.bass.wav`: il brano senza basso e il basso da solo (con `-stems`).

La prima volta scarica due cose, che restano nella cache dell'utente: la libreria ONNX Runtime (circa 30 MB, dal rilascio ufficiale su GitHub) e il modello Demucs (174 MB, dal sito di Manico). Entrambe sono verificate con il loro hash.

Opzioni principali: `-tuning 4|5|5c|6`, `-frets 12`, `-sensitivity 0.72` (più alta se mancano note ribattute, più bassa se una nota tenuta viene spezzata), `-beats 4`, `-mix` (senza separazione: più veloce, molto meno preciso). `cbass help` le elenca tutte.

Legge MP3 e WAV. Il programma non invia nulla in rete: scarica soltanto libreria e modello.

## Come è fatto

| Pacchetto | Cosa fa |
|---|---|
| `internal/audio` | lettura di WAV e MP3, ricampionamento, scrittura WAV |
| `internal/demucs` | separazione in batteria, basso, altro e voce: spettrogramma, rete (ONNX Runtime), ricostruzione |
| `internal/transcribe` | le note: su un basso isolato si segue la nota (una tenuta è una nota sola), su un mix si cercano gli attacchi |
| `internal/rhythm` | tempo e beat della registrazione, note sulla griglia dei sedicesimi, valori e legature |
| `internal/fretboard` | accordature e diteggiatura |
| `internal/tab` | tablatura in testo |
| `internal/provision` | trova o scarica libreria e modello |
| `cmd/cbass` | il programma |

La parte di elaborazione del segnale attorno alla rete segue [demucs-js](https://github.com/bakkot/demucs-js) di Kevin Gibbons (MIT), con due correzioni: il riflesso a sinistra dello spettrogramma e la lunghezza della trasformata inversa, che là lasciava un vuoto alla fine di ogni finestra.

## Compilare

Serve Go 1.24 e un compilatore C (la libreria ONNX Runtime è chiamata tramite cgo).

```
go test ./...
go build -o cbass ./cmd/cbass
```

Ogni push compila per macOS arm64, Windows x64 e Linux x64, e su ciascuna macchina prova il programma intero, modello compreso, su una registrazione creata al momento (`tools/testtone`). I programmi compilati sono tra gli artefatti dell'esecuzione, nella scheda Actions.

## Licenze

Il codice è GPL-3.0-or-later. I pesi del modello derivano da quelli pubblicati da Meta per Demucs, resi disponibili per uso personale e di ricerca; non fanno parte di questo repository.

---

## English

**Status.** The engine is done and runs from the command line: bass separation (Demucs through ONNX Runtime), notes, tempo and bars, fingering, tablature. It gives the same results as Manico 7.2.0: on the test track (4:48) all 984 notes and 705 beats match one for one. The interface (playback at variable speed, scrolling tablature, fretboard) and the packaged applications for macOS (Apple Silicon) and Windows are still to do.

**Use.** `cbass analyse -stems track.mp3` writes, beside the track, `track.cbass.json` (notes, bars, fingering), `track.tab.txt` (the tablature with bars and note values) and, with `-stems`, the track without its bass and the bass alone. On first use it downloads ONNX Runtime (about 30 MB, from the official GitHub release) and the Demucs model (174 MB, from Manico's site) into the user's cache, each checked against its hash. `cbass help` lists the options.

**Build.** Go 1.24 and a C compiler. `go test ./...`, then `go build -o cbass ./cmd/cbass`. Every push is built for macOS arm64, Windows x64 and Linux x64, and on each machine the whole program, model included, is tried on a recording made on the spot; the binaries are among the run's artifacts.

**Licences.** The code is GPL-3.0-or-later. The model weights derive from Meta's Demucs weights, made available for personal and research use; they are not part of this repository.
