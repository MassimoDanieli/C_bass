# C_bass

Dal brano alla parte di basso, come programma a sé: niente browser, niente JavaScript. È [Manico](https://github.com/MassimoDanieli/accordi_di_basso) riscritto in Go.

*From a recording to a bass part, as a program of its own: no browser, no JavaScript. It is [Manico](https://github.com/MassimoDanieli/accordi_di_basso) rewritten in Go. English follows the Italian.*

## A che punto è

| | |
|---|---|
| **Motore** (separazione del basso, note, tempo e battute, diteggiatura, tablatura) | fatto |
| **Finestra** (riproduzione a velocità variabile, tablatura che scorre, manico, accordi, correzioni a mano, metronomo, esportazione) | fatta |
| **Applicazione** per macOS (Apple Silicon), Windows e Linux | fatta: si scarica dalla pagina [Releases](https://github.com/MassimoDanieli/C_bass/releases) e si installa; non è firmata |

Il motore è nato come copia fedele di quello di Manico 7.2.0. La lettura delle note è stata poi rifatta (vedi sotto); da Manico 7.3.0 i due programmi leggono le note con lo stesso codice, e da Manico 7.4.0 anche gli accordi.

**[Manico e C_bass, presentati insieme](https://basso.massimodanieli.com/about.html)** · *[in English](https://basso.massimodanieli.com/about.html#en)*

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

### Su otto canzoni intere

Otto canzoni pop e rock scritte in MIDI (il basso scritto fa da riferimento), suonate con un banco di suoni, 75 secondi ciascuna: 1621 note. `tools/proof/proof.sh` rifà la misura.

| | note giuste |
|---|---|
| il lettore, sul basso vero | 95% (98% senza la canzone il cui basso scende sotto il Mi grave) |
| tutto il programma, dal brano intero | 74% |

La differenza sta in tre canzoni. In due la separazione non trova il basso (un fretless e un basso a plettro, suonati piano dal banco di suoni): lì il lettore non ha niente da leggere, e non c'è rimedio dal lato del lettore. Nella terza le note ribattute, che il banco di suoni lega una all'altra, dopo la separazione si fondono. Nelle altre cinque il brano intero dà lo stesso risultato del basso vero, tra il 94 e il 100%. Sono strumenti campionati, non registrazioni: dicono dove il programma è fragile, non quanto è preciso su un disco.

Su queste misure ho provato anche due rimedi automatici, e li ho lasciati come comandi a mano perché non reggevano: far guidare il battito dalle note del basso aggiusta la bossa inclusa ma rompe una canzone a ottavi ribattuti; un lettore più sensibile ritrova metà delle note ribattute fuse, ma su un basso vero aggiunge note che senza ascoltare non si distinguono da note spezzate.

Le battute: il tempo è giusto in sette su otto (una canzone a 175 è contata a 87: sopra la tablatura ci sono ÷2 e ×2), e la prima battuta cade sull'«uno» in sei delle sette, dopo che il programma ha imparato a distinguere l'uno dal tre guardando dove cambiano gli accordi e il basso (prima: quattro su sette). Dove i levare sono più forti dei battere, come in una bossa, il battito può essere seguito mezzo quarto in ritardo.

## Installare

Dalla pagina [Releases](https://github.com/MassimoDanieli/C_bass/releases):

| | |
|---|---|
| **macOS** (Apple Silicon) | `C_bass-macos-arm64.dmg`: si apre e si trascina C_bass in Applicazioni |
| **Windows** (64 bit) | `C_bass-windows-x64-setup.exe`: l'installer, solo per il proprio utente, senza password di amministratore. `C_bass-windows-x64-portable.exe` parte senza installare niente |
| **Linux** (64 bit) | `C_bass-linux-x64.deb` per Debian, Ubuntu, Mint: `sudo apt install ./C_bass-linux-x64.deb`. `C_bass-linux-x64.tar.gz` per le altre: si scompatta e si lancia `./install.sh`, che installa nella propria home (`--remove` per toglierlo) |

Nessuno è firmato con un certificato a pagamento, quindi al primo avvio il sistema chiede conferma. macOS: Impostazioni di Sistema → Privacy e sicurezza → «Apri comunque». Windows: «Ulteriori informazioni» → «Esegui comunque».

## L'applicazione

Si apre `C_bass` e si trascina un brano nella finestra, oppure lo si sceglie con «Scegli i file…». Se ne possono dare tanti insieme, o una cartella intera (anche con sottocartelle): vengono analizzati uno dopo l'altro, quelli già fatti si saltano, e alla fine la lista dice quali sono pronti e quali non sono riusciti. Lo stesso vale per la riga di comando: `cbass cartella` o `cbass a.mp3 b.mp3`. MP3, WAV e FLAC si leggono senza altro; M4A, AAC, Ogg e AIFF passano per `afconvert` su macOS (c'è già) o per `ffmpeg` su Windows e Linux (va installato). La prima volta il brano viene analizzato (circa un minuto, e si può annullare); poi resta nell'elenco e si riapre subito.

Nell'elenco ci sono già cinque brani scritti per il programma, con basso e batteria già separati: un blues in Mi, un funk in Mi, una bossa nova in La, un walking in Fa e un rock in Sol. Servono a provare la finestra e a farci le mani; si possono togliere.

La finestra è in italiano o in inglese: segue la lingua del computer, e il pulsante in alto a destra la cambia.

Durante la riproduzione:

- la **tablatura** scorre sotto una linea ferma: un numero la raggiunge quando la sua nota suona. Una nota tenuta è scritta una volta sola, con una linea lunga quanto dura; sotto il rigo ci sono i valori (gambo, tagli, punto);
- il **manico** mostra la sola nota da suonare: piena mentre suona, un cerchio vuoto per la prossima;
- **velocità** dal 40% al 120% senza cambiare l'intonazione, e senza ribattere le note: l'attacco passa una volta sola, il tempo si recupera nella nota che suona;
- **ripeti** da una battuta a un'altra;
- volume separato per il **basso** e per **il resto**: basso a zero per suonarci sopra, il resto a zero per sentire solo il basso;
- strumento a 4, 5 o 6 corde: la diteggiatura si ricalcola; con «primi tasti» resta vicino al capotasto dove la linea lo permette, con «tutto il manico» va dove la mano si sposta di meno;
- gli **accordi** sono scritti sopra le battute: il programma li legge dalla base (maggiore, minore, settima, minore settima, settima maggiore), con l'aiuto del basso per la fondamentale. Finora la lettura è stata verificata solo sui cinque brani inclusi (fondamentali tutte giuste, una settima letta come maggiore semplice): su registrazioni vere va presa come una proposta da correggere;
- **stanghette** spostabili di un quarto, quando l'«uno» è stato sentito nel punto sbagliato;
- una battuta può avere una **lunghezza sua** (una battuta in 2/4 dentro un brano in 4/4): i pulsanti − e + in alto a destra nella tablatura cambiano quella in cui ci si trova, e le stanghette dopo si spostano di conseguenza;
- una nota di cui il programma non è sicuro (smorzata, o brevissima) porta un **?**;
- se nel brano il basso non c'è (una base per suonarci sopra), lo dice invece di inventare note.

**Correggere.** Un clic su una nota la sceglie: sotto la tablatura compaiono i pulsanti per alzarla o abbassarla di un semitono, spostarla su un'altra corda, allungarla o accorciarla, anticiparla o ritardarla di un sedicesimo, toglierla. «+ Nota» ne mette una nel punto in cui ci si trova. Un clic su un accordo permette di cambiarne la fondamentale e il tipo o di toglierlo; «+ Accordo» ne aggiunge uno. «Annulla» (o Cmd/Ctrl+Z) torna indietro, fino a cento passi. Le correzioni restano salvate con il brano.

**Studiare.** «Conta una battuta» fa sentire una battuta di clic prima che il brano parta; «Metronomo» batte i quarti sul tempo vero della registrazione, più forte sull'uno; «Più veloce a ogni giro» alza la velocità del 5% a ogni ripetizione del tratto scelto, fino al 100%: si parte lenti e si arriva a tempo.

**Tonalità.** «Tonalità − +» sposta tutto il brano di un semitono alla volta, fino a sei: la registrazione suona più alta o più bassa alla velocità di prima, e note e accordi sono scritti dove ora suonano. Serve quando il brano è registrato mezzo tono sotto, o per portarlo dove canta chi lo canta.

**Sezioni.** «+ Sezione» fa cominciare una sezione (intro, strofa, ritornello, ponte, solo, finale) dalla battuta in cui ci si trova; un clic sul nome la rinomina, la ripete per intero o la toglie; `[` e `]` saltano da una all'altra.

**Quando il programma sbaglia in blocco.** Tre comandi sopra e sotto la tablatura: «+½» sposta il battito di mezzo quarto, per i brani in cui è stato seguito sui levare (lo si sente dal metronomo che batte in mezzo); «÷2» e «×2» per il tempo contato al doppio o alla metà; «Ribattute» rilegge le note più pronto a prendere per nuova una nota ribattuta, per quando di più note uguali ne è stata scritta una lunga. Se il basso scende sotto il Mi grave, va scelto lo strumento a cinque corde.

**Esportare.** «Esporta» scrive la parte nella cartella dei download: un **PDF** da stampare (tablatura, valori, accordi, sezioni), un file **MusicXML** che Guitar Pro, MuseScore e simili aprono, un file **MIDI** che tiene il tempo della registrazione (in un sequencer le note cadono dove il basso le suona), o la tablatura in **testo**.

Tasti: spazio suona e ferma, ← → una battuta indietro e avanti, ↑ ↓ velocità, A e B inizio e fine della ripetizione, L la accende e la spegne, M basso muto, K metronomo, Esc torna all'elenco. Con una nota scelta: ↑ ↓ un semitono, Maiusc+↑ ↓ cambia corda, ← → passa alla nota vicina, Maiusc+← → la sposta, − e = la accorcia e la allunga, Canc la toglie, Esc la lascia.

Quando niente si muove la finestra non viene ridisegnata: ferma, non consuma.

**Aiuto e informazioni.** «Aiuto» (o F1, o il «?» mentre si suona) elenca comandi e tasti; «Informazioni» dice versione, licenza, da dove vengono le parti del programma e cosa resta sul computer, e apre la cartella dei brani e il registro.

**Nuove versioni.** All'avvio il programma chiede a GitHub quali versioni sono state pubblicate e, se ce n'è una più nuova, lo dice nella schermata iniziale. È una sola richiesta a una pagina pubblica, senza niente del computer o dei brani; si spegne da «Informazioni».

I brani analizzati stanno nella cartella dell'utente (`~/Library/Application Support/C_bass` su macOS, `%AppData%\C_bass` su Windows, `~/.config/C_bass` su Linux); il file originale non viene toccato.

## Da riga di comando

```
cbass -stems brano.mp3
```

Scrive accanto al brano:

- `brano.cbass.json`: note, battute e diteggiatura;
- `brano.tab.txt`: la tablatura, con battute e valori ritmici;
- `brano.no-bass.wav` e `brano.bass.wav`: il brano senza basso e il basso da solo (con `-stems`).

Opzioni principali: `-tuning 4|5|5c|6`, `-frets 12`, `-sensitivity 0.72` (più alta se mancano note ribattute, più bassa se una nota tenuta viene spezzata), `-beats 4`, `-mix` (senza separazione: più veloce, molto meno preciso). `cbass help` le elenca tutte.

## Memoria e registro

La separazione è la parte pesante: circa 2,5–3,5 GB di memoria per un brano di qualche minuto, 4 GB per uno di nove. Su un computer con 8 GB va bene; con 4 no.

La finestra tiene un registro accanto alle impostazioni (`log.txt`): ogni passo con il suo tempo, e cosa non ha funzionato. Resta sul computer. La schermata d'errore lo apre, e apre la pagina per segnalare un problema.

## Cosa scarica

La prima volta, applicazione e riga di comando scaricano due cose, che restano nella cache dell'utente: la libreria ONNX Runtime (circa 30 MB, dal rilascio ufficiale su GitHub) e il modello Demucs (174 MB, dal sito di Manico). Entrambe sono verificate con il loro hash. I brani non vengono mai mandati in rete; l'unica altra richiesta è quella, descritta sopra, che all'avvio chiede a GitHub se c'è una versione nuova.

## Come è fatto

| Pacchetto | Cosa fa |
|---|---|
| `internal/audio` | lettura di WAV, MP3 e FLAC (gli altri formati con l'aiuto del sistema), ricampionamento, scrittura WAV |
| `internal/demucs` | separazione in batteria, basso, altro e voce: spettrogramma, rete (ONNX Runtime), ricostruzione |
| `internal/transcribe` | le note: su un basso isolato si segue la nota (una tenuta è una nota sola) e la sua altezza si legge dall'intera nota, non momento per momento; su un mix si cercano gli attacchi |
| `internal/rhythm` | tempo e beat della registrazione, battute anche di lunghezza diversa, note sulla griglia dei sedicesimi, valori e legature |
| `internal/chords` | gli accordi, dalla base e dal basso |
| `internal/fretboard` | accordature e diteggiatura |
| `internal/tab` | tablatura in testo |
| `internal/export` | PDF e MusicXML |
| `internal/provision` | trova o scarica libreria e modello |
| `internal/project` | tutto il percorso, dal brano alla parte: lo usano riga di comando e finestra |
| `internal/library` | i brani già analizzati |
| `internal/player` | riproduzione: due tracce, velocità variabile a intonazione ferma (WSOLA), ripetizione, metronomo e conto iniziale |
| `internal/demo` | i cinque brani inclusi: scritti in codice, suonati da strumenti fatti in codice (basso elettrico, batteria, organo, piano elettrico, chitarra classica) |
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

Ogni push compila per macOS arm64, Windows x64 e Linux x64, e su ciascuna macchina prova il programma intero, modello compreso, su una registrazione creata al momento (`tools/testtone`). Poi compila l'applicazione, la apre su uno dei brani inclusi e ne conserva un'immagine, e ne fa i pacchetti: immagine disco per macOS, installer per Windows (Inno Setup, `packaging/C_bass.iss`), `.deb` e archivio per Linux (`packaging/linux.sh`). Ogni pacchetto è provato lì: installato, avviato, tolto. Lanciata a mano con un nome di versione, la build raccoglie i pacchetti in una bozza di release.

## Licenze

Il codice è GPL-3.0-or-later. Il modello che separa il basso non fa parte di questo repository e ha condizioni sue: da dove viene e come lo si può usare è scritto in [MODEL.md](MODEL.md). Per segnalare un problema o proporre una modifica: [CONTRIBUTING.md](CONTRIBUTING.md). La finestra usa [Ebitengine](https://ebitengine.org) (Apache-2.0) e i caratteri Go (BSD).

---

## English

**Status.** The engine is done: bass separation (Demucs through ONNX Runtime), notes, tempo and bars, fingering, tablature. It began as a faithful port of Manico 7.2.0; the note reader has since been redone, and since Manico 7.3.0 both programs read the notes with the same code (the chords too, since 7.4.0). Measured against a score (a 3:21 recording whose bass is written in a Guitar Pro file, 155 notes): 148 right, 5 an octave off, 2 wrong and 39 extra notes before; 155 right and none extra now. That recording is a synthetic bass, the easy case; on a plucked line with repeats, octaves and muted notes from 90 to 170 BPM it went from 91–93% to 100%. The pitch is now read from the whole note rather than frame by frame, a note is not split where only its sound changes, pitch is followed between semitones, and the separated bass is measured against the recording so that what is left of no bass is not read as notes. The applications are on the [Releases](https://github.com/MassimoDanieli/C_bass/releases) page: a disk image for macOS (Apple Silicon), an installer and a portable program for Windows, a `.deb` and an archive with `./install.sh` for Linux. None is signed with a paid certificate, so the system asks before the first start (macOS: System Settings → Privacy & Security → Open Anyway; Windows: More info → Run anyway).

**Several recordings.** Drop many files or a whole folder (folders within it included) on the window, or pick them with "Choose files…" and "Choose a folder…": they are analysed one after the other, the ones already done are skipped, and at the end the list says which are ready and which did not work. The command line does the same: `cbass folder` or `cbass a.mp3 b.mp3`.

**The application.** Five pieces written for the program come with it (a blues, a funk, a bossa nova, a walking line, a rock), bass and drums already apart, to try the window and practise on. The window is in Italian or English, following the computer's language, with a button to switch. Open `C_bass` and drop a recording on the window: MP3, WAV and FLAC are read directly; M4A, AAC, Ogg and AIFF go through `afconvert` on macOS (already there) or `ffmpeg` on Windows and Linux (to be installed). The first time it is analysed, which takes about a minute and can be cancelled; after that it stays in the list and opens at once. While it plays, the tablature scrolls under a fixed line (a held note is written once, with a line as long as it lasts, and note values under the staff), the neck shows the one note to play, the speed goes from 40% to 120% without changing the pitch and without striking notes twice (an attack goes by once, and the time is made up in the note ringing after it), a stretch of bars can be repeated, and the bass and the rest each have their own volume. The chords are written above the bars, read from the backing with the help of the bass for the root (major, minor, seventh, minor seventh, major seventh); so far this has only been checked on the five built-in pieces (every root right, one seventh read as a plain major), so on real recordings take it as a proposal to correct. The fingering can be kept near the nut ("first frets") or left to go where the hand moves least ("whole neck"). The bar lines can be moved by a beat, and a single bar can be given its own length (a 2/4 bar in a 4/4 piece) with the − and + buttons at the top right of the tablature. A note the reader is unsure of carries a question mark, and a recording with no bass in it is said to have none rather than given made-up notes. *Correcting:* click a note to choose it, then raise or lower it by a semitone, move it to another string, make it longer or shorter, earlier or later by a sixteenth, or remove it; "+ Note" adds one where you are. Click a chord to change its root and kind or remove it; "+ Chord" adds one. "Undo" (or Cmd/Ctrl+Z) goes back, up to a hundred steps. Corrections are saved with the recording. *Practising:* "Count a bar in" plays a bar of clicks before the recording starts, "Metronome" clicks the beats on the recording's real tempo, and "Faster every time round" raises the speed by 5% at each pass of the repeated stretch, up to 100%. *Key:* "Key − +" moves the whole piece a semitone at a time, up to six: the recording plays higher or lower at the speed it had, and notes and chords are written where they now sound. *Sections:* "+ Section" starts a section (intro, verse, chorus, bridge, solo, outro) at the bar you are in; click its name to rename it, repeat it whole or remove it; `[` and `]` jump from one to the next. *When the program is wrong wholesale:* "+½" moves the beat half a beat later, for pieces where it was followed on the off-beats; "÷2" and "×2" for a tempo counted at twice or half; "Repeated notes" reads the notes again, readier to take a repeated note for a new one; for a bass that goes below low E, choose the five-string. *Exporting:* "Export" writes the part to the Downloads folder as a PDF to print, as MusicXML for Guitar Pro, MuseScore and the like, as a MIDI file that keeps the time of the recording, or as text tablature. Keys: space to play and stop, ← → a bar back and on, ↑ ↓ speed, A and B the start and end of the repeat, L to turn it on and off, M to mute the bass, K the metronome, Esc back to the list; with a note chosen, ↑ ↓ a semitone, Shift+↑ ↓ another string, ← → the next note, Shift+← → move it, − and = shorter and longer, Delete removes it, Esc lets it go. A window in which nothing moves is not redrawn. Analysed recordings are kept in the user's folder (`~/Library/Application Support/C_bass` on macOS, `%AppData%\C_bass` on Windows).

**On eight whole songs.** Eight pop and rock songs written as MIDI (the written bass is the reference), played with a sound font, 75 seconds each, 1621 notes; `tools/proof/proof.sh` repeats the measurement. The reader on the true bass gets 95% of the notes right (98% leaving out the song whose bass goes below low E); the whole program, from the full recording, 74%. The gap is three songs: in two the separation does not find the bass (a fretless and a picked bass, played softly by the sound font), in the third repeated notes, which the sound font ties together, merge after separation; in the other five the full recording gives the same as the true bass, 94 to 100%. These are sampled instruments, not recordings: they show where the program is fragile, not how accurate it is on a record. The tempo is right in seven of eight (one song at 175 is counted at 87: the tablature has ÷2 and ×2), and the first bar falls on the "one" in six of those seven, now that the first beat is told from the third by where the chords and the bass change (before: four of seven). Where the off-beats are louder than the beats, as in a bossa, the beat may be followed half a beat late.

**Memory and log.** Separating takes about 2.5–3.5 GB of memory for a recording of a few minutes, 4 GB for one of nine. The window keeps a log beside its settings (`log.txt`): each step with its time, and what failed; it stays on the computer, and the error screen opens it and the page for reporting a problem.

**Command line.** `cbass -stems track.mp3` writes, beside the track, `track.cbass.json` (notes, bars, fingering), `track.tab.txt` (the tablature with bars and note values) and, with `-stems`, the track without its bass and the bass alone. `cbass help` lists the options.

**Downloads.** On first use ONNX Runtime (about 30 MB, from the official GitHub release) and the Demucs model (174 MB, from Manico's site) are fetched into the user's cache, each checked against its hash. The recordings are never sent anywhere. At start the program asks GitHub which versions have been published, to say when there is a newer one: one request for a public page, with nothing about the computer or the recordings, which the About page turns off. "Help" (or F1, or the "?" while playing) lists controls and keys; "About" gives version, licence, where the parts of the program come from and what stays on the computer.

**Build.** Go 1.26 and a C compiler; on Linux the window also needs the X11, OpenGL and ALSA headers listed in `.github/workflows/build.yml`. `go test ./...`, then `go build -o cbass ./cmd/cbass` and `go build -o cbass-app ./cmd/cbass-app`. Every push is built for macOS arm64, Windows x64 and Linux x64, and on each machine the whole program, model included, is tried on a recording made on the spot; then the application is built, opened on one of its built-in pieces with a picture of the window kept, and packed (disk image, Inno Setup installer from `packaging/C_bass.iss`, `.deb` and archive from `packaging/linux.sh`); each package is tried there: installed, started, removed. Run by hand with a version name, the build gathers the packages in a draft release.

**Licences.** The code is GPL-3.0-or-later. The model that separates the bass is not part of this repository and has terms of its own: where it comes from and how it may be used is in [MODEL.md](MODEL.md). To report a problem or propose a change: [CONTRIBUTING.md](CONTRIBUTING.md). The window uses [Ebitengine](https://ebitengine.org) (Apache-2.0) and the Go fonts (BSD).
