# C_bass

Dal brano alla parte di basso, come programma a sé: niente browser, niente JavaScript. È [Manico](https://github.com/MassimoDanieli/accordi_di_basso) riscritto in Go.

*From a recording to a bass part, as a program of its own: no browser, no JavaScript. It is [Manico](https://github.com/MassimoDanieli/accordi_di_basso) rewritten in Go. English follows the Italian.*

## A che punto è

| | |
|---|---|
| **Motore** (separazione del basso, note, tempo e battute, diteggiatura, tablatura) | fatto |
| **Finestra** (riproduzione a velocità variabile, tablatura che scorre, manico) | fatta, prima versione |
| **Applicazione** per macOS (Apple Silicon) e Windows | si scarica dalla scheda Actions; manca l'installazione |

Il motore è nato come copia fedele di quello di Manico 7.2.0. La lettura delle note è stata poi rifatta (vedi sotto) e non coincide più con quella di Manico.

### Quanto è precisa la lettura delle note

Misurata contro uno spartito: una registrazione di 3:21 il cui basso è scritto in un file Guitar Pro (155 note), separata e letta dal programma.

| | prima | adesso |
|---|---|---|
| note giuste | 148 | 155 |
| ottava sbagliata | 5 | 0 |
| nota sbagliata | 2 | 0 |
| note in più (una nota spezzata in due, o inventata) | 39 | 0 |

Quella registrazione è un basso sintetico, quindi facile. Su una linea pizzicata con ribattuti, ottave e note smorzate, da 90 a 170 BPM: dal 91–93% al 100%. Su un basso vero non c'è uno spartito con cui misurare; lì le note lette scendono da 846 a 687 perché non si spezzano più, e quelle sotto gli 80 ms da 131 a 62.

Cosa è cambiato: l'altezza si legge dall'intera nota e non da ogni centesimo di secondo (un momento solo confonde facilmente un'ottava con l'altra); una nota non si spezza dove cambia solo il suo suono (una corda bassa che risuona perde la fondamentale); l'altezza si segue tra un semitono e l'altro, così un basso un po' calante o un fretless non sfarfalla tra due nomi e un glissato non lascia una nota a ogni tasto; il basso separato si misura contro il brano, così il fruscio che la separazione lascia dove il basso non c'è non diventa note.

## L'applicazione

Si apre `C_bass` e si trascina un brano (MP3 o WAV) nella finestra, oppure lo si sceglie con «Scegli un file…». La prima volta il brano viene analizzato (circa un minuto); poi resta nell'elenco e si riapre subito.

Nell'elenco ci sono già cinque brani scritti per il programma, con basso e batteria già separati: un blues in Mi, un funk in Mi, una bossa nova in La, un walking in Fa e un rock in Sol. Servono a provare la finestra e a farci le mani; si possono togliere.

La finestra è in italiano o in inglese: segue la lingua del computer, e il pulsante in alto a destra la cambia.

Durante la riproduzione:

- la **tablatura** scorre sotto una linea ferma: un numero la raggiunge quando la sua nota suona. Una nota tenuta è scritta una volta sola, con una linea lunga quanto dura; sotto il rigo ci sono i valori (gambo, tagli, punto);
- il **manico** mostra la sola nota da suonare: piena mentre suona, un cerchio vuoto per la prossima;
- **velocità** dal 40% al 120% senza cambiare l'intonazione, e senza ribattere le note: l'attacco passa una volta sola, il tempo si recupera nella nota che suona;
- **ripeti** da una battuta a un'altra;
- volume separato per il **basso** e per **il resto**: basso a zero per suonarci sopra, il resto a zero per sentire solo il basso;
- strumento a 4, 5 o 6 corde: la diteggiatura si ricalcola;
- **stanghette** spostabili di un quarto, quando l'«uno» è stato sentito nel punto sbagliato;
- una nota di cui il programma non è sicuro (smorzata, o brevissima) porta un **?**;
- se nel brano il basso non c'è (una base per suonarci sopra), lo dice invece di inventare note.

Tasti: spazio suona e ferma, ← → una battuta indietro e avanti, ↑ ↓ velocità, A e B inizio e fine della ripetizione, L la accende e la spegne, M basso muto, Esc torna all'elenco.

I brani analizzati stanno nella cartella dell'utente (`~/Library/Application Support/C_bass` su macOS, `%AppData%\C_bass` su Windows); il file originale non viene toccato.

## Da riga di comando

```
cbass -stems brano.mp3
```

Scrive accanto al brano:

- `brano.cbass.json`: note, battute e diteggiatura;
- `brano.tab.txt`: la tablatura, con battute e valori ritmici;
- `brano.no-bass.wav` e `brano.bass.wav`: il brano senza basso e il basso da solo (con `-stems`).

Opzioni principali: `-tuning 4|5|5c|6`, `-frets 12`, `-sensitivity 0.72` (più alta se mancano note ribattute, più bassa se una nota tenuta viene spezzata), `-beats 4`, `-mix` (senza separazione: più veloce, molto meno preciso). `cbass help` le elenca tutte.

## Cosa scarica

La prima volta, applicazione e riga di comando scaricano due cose, che restano nella cache dell'utente: la libreria ONNX Runtime (circa 30 MB, dal rilascio ufficiale su GitHub) e il modello Demucs (174 MB, dal sito di Manico). Entrambe sono verificate con il loro hash. Il programma non invia nulla in rete.

Legge MP3 e WAV.

## Come è fatto

| Pacchetto | Cosa fa |
|---|---|
| `internal/audio` | lettura di WAV e MP3, ricampionamento, scrittura WAV |
| `internal/demucs` | separazione in batteria, basso, altro e voce: spettrogramma, rete (ONNX Runtime), ricostruzione |
| `internal/transcribe` | le note: su un basso isolato si segue la nota (una tenuta è una nota sola) e la sua altezza si legge dall'intera nota, non momento per momento; su un mix si cercano gli attacchi |
| `internal/rhythm` | tempo e beat della registrazione, note sulla griglia dei sedicesimi, valori e legature |
| `internal/fretboard` | accordature e diteggiatura |
| `internal/tab` | tablatura in testo |
| `internal/provision` | trova o scarica libreria e modello |
| `internal/project` | tutto il percorso, dal brano alla parte: lo usano riga di comando e finestra |
| `internal/library` | i brani già analizzati |
| `internal/player` | riproduzione: due tracce, velocità variabile a intonazione ferma (WSOLA), ripetizione |
| `internal/demo` | i cinque brani inclusi: scritti in codice, suonati da un sintetizzatore a corda pizzicata |
| `internal/app` | la finestra ([Ebitengine](https://ebitengine.org)) |
| `cmd/cbass`, `cmd/cbass-app` | la riga di comando e l'applicazione |

La parte di elaborazione del segnale attorno alla rete segue [demucs-js](https://github.com/bakkot/demucs-js) di Kevin Gibbons (MIT), con due correzioni: il riflesso a sinistra dello spettrogramma e la lunghezza della trasformata inversa, che là lasciava un vuoto alla fine di ogni finestra.

## Compilare

Serve Go 1.26 e un compilatore C (la libreria ONNX Runtime è chiamata tramite cgo). Su Linux la finestra vuole anche le intestazioni di X11, OpenGL e ALSA (l'elenco è in `.github/workflows/build.yml`).

```
go test ./...
go build -o cbass ./cmd/cbass
go build -o cbass-app ./cmd/cbass-app
```

Ogni push compila per macOS arm64, Windows x64 e Linux x64, e su ciascuna macchina prova il programma intero, modello compreso, su una registrazione creata al momento (`tools/testtone`). Su macOS e Windows compila anche l'applicazione, la apre su un brano inventato (`tools/demosong`) e ne conserva un'immagine. Programmi, applicazioni e immagini sono tra gli artefatti dell'esecuzione, nella scheda Actions.

L'applicazione per macOS non è firmata con un certificato Apple: se è stata scaricata dal browser, la prima volta va aperta con clic destro, «Apri».

## Licenze

Il codice è GPL-3.0-or-later. I pesi del modello derivano da quelli pubblicati da Meta per Demucs, resi disponibili per uso personale e di ricerca; non fanno parte di questo repository. La finestra usa [Ebitengine](https://ebitengine.org) (Apache-2.0) e i caratteri Go (BSD).

---

## English

**Status.** The engine is done: bass separation (Demucs through ONNX Runtime), notes, tempo and bars, fingering, tablature. It began as a faithful port of Manico 7.2.0; the note reader has since been redone. Measured against a score (a 3:21 recording whose bass is written in a Guitar Pro file, 155 notes): 148 right, 5 an octave off, 2 wrong and 39 extra notes before; 155 right and none extra now. That recording is a synthetic bass, the easy case; on a plucked line with repeats, octaves and muted notes from 90 to 170 BPM it went from 91–93% to 100%. The pitch is now read from the whole note rather than frame by frame, a note is not split where only its sound changes, pitch is followed between semitones, and the separated bass is measured against the recording so that what is left of no bass is not read as notes. The window is in its first version; the applications for macOS (Apple Silicon) and Windows are built on every push and downloaded from the Actions tab, with no installer yet.

**The application.** Five pieces written for the program come with it (a blues, a funk, a bossa nova, a walking line, a rock), bass and drums already apart, to try the window and practise on. The window is in Italian or English, following the computer's language, with a button to switch. Open `C_bass` and drop a recording (MP3 or WAV) on the window. The first time it is analysed, which takes about a minute; after that it stays in the list and opens at once. While it plays, the tablature scrolls under a fixed line (a held note is written once, with a line as long as it lasts, and note values under the staff), the neck shows the one note to play, the speed goes from 40% to 120% without changing the pitch and without striking notes twice (an attack goes by once, and the time is made up in the note ringing after it), a stretch of bars can be repeated, and the bass and the rest each have their own volume. The bar lines can be moved by a beat, a note the reader is unsure of carries a question mark, and a recording with no bass in it is said to have none rather than given made-up notes. Keys: space to play and stop, ← → a bar back and on, ↑ ↓ speed, A and B the start and end of the repeat, L to turn it on and off, M to mute the bass, Esc back to the list. Analysed recordings are kept in the user's folder (`~/Library/Application Support/C_bass` on macOS, `%AppData%\C_bass` on Windows).

**Command line.** `cbass -stems track.mp3` writes, beside the track, `track.cbass.json` (notes, bars, fingering), `track.tab.txt` (the tablature with bars and note values) and, with `-stems`, the track without its bass and the bass alone. `cbass help` lists the options.

**Downloads.** On first use ONNX Runtime (about 30 MB, from the official GitHub release) and the Demucs model (174 MB, from Manico's site) are fetched into the user's cache, each checked against its hash. Nothing is sent anywhere.

**Build.** Go 1.26 and a C compiler; on Linux the window also needs the X11, OpenGL and ALSA headers listed in `.github/workflows/build.yml`. `go test ./...`, then `go build -o cbass ./cmd/cbass` and `go build -o cbass-app ./cmd/cbass-app`. Every push is built for macOS arm64, Windows x64 and Linux x64, and on each machine the whole program, model included, is tried on a recording made on the spot; on macOS and Windows the application is built too, opened on a made-up recording, and a picture of its window kept. The macOS application is not signed with an Apple certificate: if it came through a browser, open it the first time with right-click, Open.

**Licences.** The code is GPL-3.0-or-later. The model weights derive from Meta's Demucs weights, made available for personal and research use; they are not part of this repository. The window uses [Ebitengine](https://ebitengine.org) (Apache-2.0) and the Go fonts (BSD).
