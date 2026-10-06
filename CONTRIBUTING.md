# Contribuire a C_bass

*English follows the Italian.*

C_bass è software libero (GPL-3.0-or-later). Segnalazioni, correzioni e idee sono benvenute; qui c'è quello che serve sapere per farlo senza perdere tempo.

## Segnalare un problema

Apri una [segnalazione](https://github.com/MassimoDanieli/C_bass/issues/new/choose). Aiuta molto sapere:

- la versione (è nella pagina «Informazioni») e il sistema;
- cosa hai fatto e cosa è successo, anche in due righe;
- il registro: la schermata d'errore e la pagina «Informazioni» lo aprono. È un file di testo con i passi fatti e l'errore; contiene i titoli dei brani che hai aperto, quindi guardalo prima di allegarlo.

Se il programma ha letto male un brano, la segnalazione più utile è quella che si può rifare: quale brano (titolo e autore bastano, non allegare musica protetta), in che punto, cosa c'è scritto e cosa dovrebbe esserci.

## Compilare e provare

Serve Go 1.26 e un compilatore C. Su Linux anche le intestazioni di X11, OpenGL e ALSA elencate in `.github/workflows/build.yml`.

```
go test ./...
go build -o cbass ./cmd/cbass
go build -o cbass-app ./cmd/cbass-app
```

Prima di proporre una modifica: `gofmt -l .` non deve stampare niente, `go vet ./...` e `go test ./...` devono passare. La build su GitHub rifà tutto su macOS, Windows e Linux.

La finestra si può fotografare senza guardarla, che è il modo in cui è stata controllata finora:

```
CBASS_NO_AUDIO=1 CBASS_SHOT=/tmp/finestra.png CBASS_OPEN=demo-blues CBASS_SHOT_AT=8 ./cbass-app
```

## Toccare il lettore delle note, il tempo o gli accordi

Queste parti si giudicano con le misure, non a occhio. `tools/proof/proof.sh` suona otto canzoni scritte in MIDI con un banco di suoni e conta le note lette contro quelle scritte; i numeri di partenza sono nel README. Una modifica che ne migliora una e ne peggiora un'altra va detta com'è: due rimedi automatici sono già stati provati e lasciati come comandi a mano proprio per questo (vedi il README, «Su otto canzoni intere»).

Chi ha registrazioni vere con lo spartito del basso e il diritto di usarle farebbe al progetto il regalo più grande: è la misura che manca.

## Come è scritto il codice

- Un pacchetto per ogni cosa che il programma fa (`internal/…`, l'elenco è nel README); `internal/app` è la finestra e non contiene elaborazione del segnale.
- I commenti dicono perché, in inglese semplice. I nomi sono parole intere.
- Ogni comportamento nuovo arriva con la sua prova. Dove il risultato è un suono, la prova lo misura (altezza, durata, posizione dei colpi): nessuno dei test ha bisogno di orecchie.
- Testi della finestra sempre nelle due lingue: `g.t("italiano", "english")`.

## Proporre una modifica

Un ramo, una pull request, una cosa per volta. Nella descrizione: cosa cambia per chi usa il programma, e come l'hai verificato. Le modifiche proposte si intendono offerte con la stessa licenza del progetto.

---

## English

C_bass is free software (GPL-3.0-or-later). Reports, fixes and ideas are welcome.

**Reporting a problem.** Open an [issue](https://github.com/MassimoDanieli/C_bass/issues/new/choose) with the version (on the About page), your system, what you did and what happened. The log helps: the error screen and the About page open it. It is a text file with the steps taken and the error; it holds the titles of the recordings you opened, so look at it before attaching it. If a recording was read wrongly, say which one (title and artist are enough; do not attach copyrighted music), where, what was written and what should have been.

**Building and testing.** Go 1.26 and a C compiler; on Linux also the X11, OpenGL and ALSA headers listed in `.github/workflows/build.yml`. `go test ./...`, `go build -o cbass ./cmd/cbass`, `go build -o cbass-app ./cmd/cbass-app`. Before proposing a change, `gofmt -l .` must print nothing and `go vet ./...` and `go test ./...` must pass. The window can be photographed without looking at it: `CBASS_NO_AUDIO=1 CBASS_SHOT=/tmp/window.png CBASS_OPEN=demo-blues CBASS_SHOT_AT=8 ./cbass-app`.

**Touching the note reader, the tempo or the chords.** These are judged by measurement. `tools/proof/proof.sh` plays eight songs written as MIDI through a sound font and counts the notes read against the notes written; the starting figures are in the README. A change that helps one song and hurts another should be reported as that. Real recordings with a written bass part, and the right to use them, are the measurement the project lacks.

**How the code is written.** One package for each thing the program does (`internal/…`); `internal/app` is the window and does no signal processing. Comments say why, in plain English; names are whole words. Every new behaviour comes with its test, and where the result is a sound the test measures it. Text in the window is always in both languages: `g.t("italiano", "english")`.

**Proposing a change.** One branch, one pull request, one thing at a time; say what changes for whoever uses the program and how you checked it. Contributions are offered under the project's licence.
