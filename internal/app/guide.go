package app

import (
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// The guide is two pages over whatever screen is open: how the program is used, and what it
// is, who made its parts, and what it does on the computer.

const homePage = "https://github.com/MassimoDanieli/C_bass"

// wrap breaks a text into lines no wider than the room.
func (c *canvas) wrap(text string, size float32, w weight, room float32) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		longer := word
		if line != "" {
			longer = line + " " + word
		}
		if line != "" && c.width(longer, size, w) > room {
			lines = append(lines, line)
			line = word
			continue
		}
		line = longer
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// part is a heading with its rows: a key or a name on the left, what it does on the right.
// A row with nothing on the left is a paragraph.
type part struct {
	title string
	rows  [][2]string
}

func (g *Game) helpParts() []part {
	t := g.t
	return []part{
		{t("Per cominciare", "To begin"), [][2]string{
			{"", t("Trascina un brano nella finestra, o sceglilo con «Scegli un file…». La prima volta viene analizzato: il basso è separato dal resto, le sue note sono lette, si cercano tempo, battute e accordi. Ci vuole circa un minuto; poi il brano resta nell'elenco e si riapre subito.",
				"Drop a recording on the window, or pick it with \"Choose a file…\". The first time it is analysed: the bass is separated from the rest, its notes are read, tempo, bars and chords are found. That takes about a minute; after that the recording stays in the list and opens at once.")},
			{"", t("La tablatura scorre sotto una linea ferma: un numero la raggiunge quando la sua nota suona. Il manico mostra la nota da suonare, piena mentre suona, un cerchio vuoto per la prossima. Una nota con un ? è una di cui il programma non è sicuro.",
				"The tablature scrolls under a fixed line: a number reaches it when its note sounds. The neck shows the note to play, filled while it sounds, an empty circle for the next one. A note with a ? is one the program is not sure of.")},
		}},
		{t("Suonare", "Playing"), [][2]string{
			{t("Spazio", "Space"), t("suona e ferma", "play and stop")},
			{"←  →", t("una battuta indietro, una avanti", "a bar back, a bar on")},
			{"↑  ↓", t("più veloce, più lento (dal 40% al 120%, l'intonazione non cambia)", "faster, slower (40% to 120%, the pitch does not change)")},
			{"A  B", t("inizio e fine del tratto da ripetere", "start and end of the stretch to repeat")},
			{"L", t("accende e spegne la ripetizione", "repeat on and off")},
			{"M", t("basso muto, per suonarci sopra", "bass muted, to play over the rest")},
			{"K", t("metronomo", "metronome")},
			{"Esc", t("torna all'elenco", "back to the list")},
		}},
		{t("Studiare", "Practising"), [][2]string{
			{t("Conta una battuta", "Count a bar in"), t("una battuta di clic prima che il brano parta", "a bar of clicks before the recording starts")},
			{t("Metronomo", "Metronome"), t("batte i quarti sul tempo vero della registrazione, più forte sull'uno", "clicks the beats on the recording's own tempo, louder on the one")},
			{t("Più veloce a ogni giro", "Faster every time round"), t("a ogni ripetizione del tratto scelto la velocità sale del 5%, fino al 100%", "at each pass of the repeated stretch the speed goes up by 5%, to 100%")},
			{t("Basso, Il resto", "Bass, The rest"), t("due volumi: basso a zero per suonarci sopra, il resto a zero per sentire solo il basso", "two volumes: bass at zero to play over the rest, the rest at zero to hear the bass alone")},
		}},
		{t("Correggere", "Correcting"), [][2]string{
			{"", t("Un clic su una nota la sceglie: sotto la tablatura compaiono i comandi per cambiarla. Un clic su un accordo fa lo stesso per l'accordo. «+ Nota» e «+ Accordo» ne aggiungono uno dove ti trovi. Le correzioni restano salvate con il brano.",
				"Click a note to choose it: the controls to change it appear under the tablature. Click a chord for the same with the chord. \"+ Note\" and \"+ Chord\" add one where you are. Corrections are saved with the recording.")},
			{"↑  ↓", t("la nota scelta sale o scende di un semitono", "the chosen note up or down a semitone")},
			{t("Maiusc ↑ ↓", "Shift ↑ ↓"), t("su un'altra corda", "to another string")},
			{"←  →", t("passa alla nota prima o dopo", "to the note before or after")},
			{t("Maiusc ← →", "Shift ← →"), t("la sposta di un sedicesimo", "moves it by a sixteenth")},
			{"−  =", t("la accorcia, la allunga", "shorter, longer")},
			{t("Canc", "Delete"), t("la toglie", "removes it")},
			{t("Ribattute", "Repeated notes"), t("rilegge le note dal basso, più o meno pronto a prendere per nuova una nota ribattuta: «di più» e «molte» quando di più note uguali ne è stata scritta una lunga", "reads the notes again from the bass, more or less ready to take a repeated note for a new one: \"more\" and \"most\" when several notes alike were written as one long one")},
			{t("Cmd/Ctrl Z", "Cmd/Ctrl Z"), t("annulla l'ultima modifica (fino a cento)", "undoes the last change (up to a hundred)")},
		}},
		{t("Battute e tempo", "Bars and tempo"), [][2]string{
			{t("stanghette ‹ ›", "bar lines ‹ ›"), t("spostano le battute di un quarto, quando l'«uno» è nel punto sbagliato", "move the bars by a beat, when the \"one\" is in the wrong place")},
			{t("battuta − +", "bar − +"), t("danno alla battuta in cui ti trovi una lunghezza sua: una battuta in 2/4 dentro un brano in 4/4", "give the bar you are in a length of its own: a 2/4 bar in a 4/4 piece")},
			{"+½", t("sposta il battito di mezzo quarto, quando è stato seguito sui levare: lo senti dal metronomo che batte in mezzo", "moves the beat half a beat later, when it was followed on the off-beats: you hear it from the metronome clicking in between")},
			{"÷2  ×2", t("contano il brano alla metà o al doppio del tempo: 87 o 174 è spesso questione di opinione", "count the piece at half or twice the tempo: 87 or 174 is often a matter of opinion")},
		}},
		{t("Strumento ed esportazione", "Instrument and export"), [][2]string{
			{t("4, 5, 6 corde", "4, 5, 6 strings"), t("la diteggiatura si ricalcola per lo strumento scelto", "the fingering is worked out again for the instrument chosen")},
			{t("primi tasti", "first frets"), t("tiene la mano vicino al capotasto dove la linea lo permette; «tutto il manico» la manda dove si sposta di meno", "keeps the hand near the nut where the line allows; \"whole neck\" sends it where it moves least")},
			{t("Esporta", "Export"), t("scrive la parte nella cartella dei download: PDF da stampare, MusicXML per Guitar Pro e MuseScore, tablatura in testo", "writes the part to the Downloads folder: a PDF to print, MusicXML for Guitar Pro and MuseScore, text tablature")},
		}},
		{t("Quando non torna", "When it is not right"), [][2]string{
			{"", t("Il programma legge meglio un basso suonato chiaro in un brano registrato bene. Sbaglia più facilmente con bassi sintetici molto gravi, note ribattute legate, e brani in cui il basso si sente appena: lì la separazione può non trovarlo. Gli accordi sono una proposta: vanno controllati a orecchio. Tutto si può correggere a mano.",
				"The program reads best a bass played clearly in a well recorded piece. It goes wrong more easily with very low synth basses, repeated notes tied together, and pieces where the bass is barely heard: there the separation may not find it. The chords are a proposal: check them by ear. Everything can be corrected by hand.")},
		}},
	}
}

// link is something on the about page that opens a page elsewhere, or a folder here.
type link struct {
	id, text, target string
}

func (g *Game) guideScreen(c *canvas) {
	if c.in != nil && g.pressed(ebiten.KeyEscape) {
		g.guide = ""
		return
	}
	width := min(g.w-80, 860)
	x := (g.w - width) / 2
	if g.button(c, "guide-back", rect{x, 20, 110, 34}, g.t("‹  Indietro", "‹  Back"), plain) {
		g.guide = ""
		return
	}
	for i, page := range []string{"help", "about"} {
		text := []string{g.t("Aiuto", "Help"), g.t("Informazioni", "About")}[i]
		l := quiet
		if g.guide == page {
			l = chosen
		}
		if g.button(c, "guide-"+page, rect{x + 130 + float32(i)*134, 20, 128, 34}, text, l) && g.guide != page {
			g.guide, g.guideScroll = page, 0
		}
	}
	if g.button(c, "language", rect{x + width - 44, 20, 44, 34}, g.t("EN", "IT"), quiet) {
		g.switchLanguage()
	}
	body := rect{x, 74, width, g.h - 74 - 16}
	if c.in != nil && body.has(c.in.x, c.in.y) {
		g.guideScroll -= c.in.wheel * 40
	}
	if c.in != nil {
		switch {
		case g.pressed(ebiten.KeyArrowDown), g.pressed(ebiten.KeyPageDown):
			g.guideScroll += body.h * 0.8
		case g.pressed(ebiten.KeyArrowUp), g.pressed(ebiten.KeyPageUp):
			g.guideScroll -= body.h * 0.8
		}
	}
	inside := c.clip(body)
	var height float32
	if g.guide == "about" {
		height = g.aboutPage(c, inside, body)
	} else {
		height = g.helpPage(inside, body)
	}
	g.guideScroll = max(0, min(g.guideScroll, max(0, height-body.h)))
	if c.painting() && height > body.h { // how far down the page is
		track := rect{body.x + body.w + 10, body.y, 4, body.h}
		c.round(track, 2, colPanel)
		c.round(rect{track.x, track.y + track.h*g.guideScroll/height, 4, track.h * body.h / height}, 2, colLine)
	}
}

// helpPage draws the help and returns how tall it is.
func (g *Game) helpPage(c *canvas, body rect) float32 {
	const column = 190 // the keys on the left
	y := body.y - g.guideScroll + 8
	start := y
	for _, p := range g.helpParts() {
		c.label(p.title, body.x, y+12, 18, bold, colText, left)
		y += 38
		for _, row := range p.rows {
			room, at := body.w-column, body.x+column
			if row[0] == "" {
				room, at = body.w, body.x
			} else {
				c.label(c.fit(row[0], 14, medium, column-16), body.x, y+9, 14, medium, colAccent2, left)
			}
			for _, line := range c.wrap(row[1], 14, regular, room) {
				c.label(line, at, y+9, 14, regular, colDim, left)
				y += 21
			}
			y += 7
		}
		y += 18
	}
	return y - start
}

// aboutPage draws what the program is and returns how tall the page is. It takes the canvas
// of the window as well as the clipped one: buttons scrolled out of sight must not be pressed.
func (g *Game) aboutPage(whole, c *canvas, body rect) float32 {
	t := g.t
	y := body.y - g.guideScroll + 8
	start := y
	paragraph := func(text string, ink color.Color) {
		for _, line := range c.wrap(text, 14, regular, body.w) {
			c.label(line, body.x, y+9, 14, regular, ink, left)
			y += 21
		}
		y += 8
	}
	heading := func(text string) {
		y += 12
		c.label(text, body.x, y+12, 18, bold, colText, left)
		y += 38
	}
	// buttons in a row, wrapped to the width
	row := func(links []link) {
		bx := body.x
		for _, l := range links {
			w := c.width(l.text, 14, medium) + 28
			if bx+w > body.x+body.w {
				bx = body.x
				y += 40
			}
			r := rect{bx, y, w, 32}
			if g.button(c, l.id, r, l.text, plain) && body.has(whole.mx, whole.my) {
				show(l.target)
			}
			bx += w + 8
		}
		y += 46
	}

	c.label("C_bass", body.x, y+16, 30, bold, colText, left)
	c.label(g.version, body.x+c.width("C_bass", 30, bold)+14, y+20, 15, regular, colDim, left)
	y += 50
	paragraph(t("Dal brano alla parte di basso: separa il basso da una registrazione, ne legge le note, trova tempo, battute e accordi, e ti fa suonare sopra con la tablatura e il manico.",
		"From a recording to a bass part: it separates the bass from a recording, reads its notes, finds tempo, bars and chords, and lets you play along with the tablature and the neck."), colDim)
	paragraph(t("Di Massimo Danieli. È software libero: puoi usarlo, studiarlo, cambiarlo e ridistribuirlo secondo la licenza GNU GPL, versione 3 o successiva. Non ha garanzie.",
		"By Massimo Danieli. It is free software: you can use it, study it, change it and pass it on under the GNU GPL, version 3 or later. It comes with no warranty."), colDim)
	row([]link{
		{"about-source", t("Il codice su GitHub", "The code on GitHub"), homePage},
		{"about-licence", t("La licenza", "The licence"), homePage + "/blob/main/LICENSE"},
		{"about-report", t("Segnala un problema", "Report a problem"), issues},
	})

	heading(t("Aggiornamenti", "Updates"))
	found, page, asked, failed := g.latest.get()
	switch {
	case g.settings.NoUpdateCheck:
		paragraph(t("Il controllo delle nuove versioni è spento.", "The check for new versions is off."), colDim)
	case found != "":
		paragraph(t("È uscita la versione ", "Version ")+found+t(".", " is out."), colAccent)
	case failed:
		paragraph(t("Non sono riuscito a controllare se c'è una versione nuova.", "Checking for a new version did not work."), colDim)
	case asked:
		paragraph(t("Questa è la versione più recente.", "This is the newest version."), colDim)
	default:
		paragraph(t("Il controllo si fa all'avvio, per le versioni pubblicate.", "The check is made at start, for published versions."), colDim)
	}
	paragraph(t("All'avvio il programma chiede a GitHub quali versioni sono state pubblicate. È una sola richiesta a una pagina pubblica: non porta con sé niente del computer, dei brani o di te.",
		"At start the program asks GitHub which versions have been published. It is one request for a public page: nothing about the computer, the recordings or you goes with it."), colFaint)
	toggle := t("Spegni il controllo", "Turn the check off")
	if g.settings.NoUpdateCheck {
		toggle = t("Accendi il controllo", "Turn the check on")
	}
	bx := body.x
	if found != "" {
		w := c.width(t("Scarica", "Download"), 14, medium) + 36
		if g.button(c, "about-download", rect{bx, y, w, 32}, t("Scarica", "Download"), primary) && body.has(whole.mx, whole.my) {
			show(page)
		}
		bx += w + 8
	}
	if g.button(c, "about-updates", rect{bx, y, c.width(toggle, 14, medium) + 28, 32}, toggle, plain) && body.has(whole.mx, whole.my) {
		g.settings.NoUpdateCheck = !g.settings.NoUpdateCheck
		g.saveSettings()
		if !g.settings.NoUpdateCheck {
			g.lookForUpdate()
		}
	}
	y += 46

	heading(t("Cosa resta sul computer", "What stays on the computer"))
	paragraph(t("Tutto. I brani che apri non vengono mandati da nessuna parte: l'analisi si fa qui. Il basso separato, il resto e la parte scritta stanno nella cartella dei brani; l'originale non viene toccato. Un registro annota cosa fa il programma e cosa non ha funzionato: resta qui anche quello.",
		"Everything. The recordings you open are not sent anywhere: the analysis is done here. The separated bass, the rest and the written part are kept in the recordings folder; the original is not touched. A log notes what the program does and what failed: that stays here too."), colDim)
	paragraph(t("Dalla rete arrivano solo, la prima volta, la libreria ONNX Runtime (circa 30 MB) e il modello che separa il basso (174 MB), ciascuno controllato contro la sua impronta.",
		"From the network come only, the first time, the ONNX Runtime library (about 30 MB) and the model that separates the bass (174 MB), each checked against its hash."), colDim)
	places := []link{{"about-library", t("Apri la cartella dei brani", "Open the recordings folder"), g.lib.Dir}}
	if path := diaryPath(); path != "" {
		places = append(places, link{"about-log", t("Apri il registro", "Open the log"), path})
	}
	row(places)

	heading(t("Fatto con", "Made with"))
	for _, credit := range [][3]string{
		{"Demucs", t("la rete che separa il basso, di Alexandre Défossez e colleghi (Meta). Il codice è MIT; i pesi sono stati allenati anche su MUSDB18, che è concesso per uso non commerciale e di ricerca.",
			"the network that separates the bass, by Alexandre Défossez and colleagues (Meta). The code is MIT; the weights were trained on MUSDB18 among others, which is licensed for non-commercial and research use."), "https://github.com/facebookresearch/demucs"},
		{"ONNX Runtime", t("fa girare la rete (Microsoft, MIT)", "runs the network (Microsoft, MIT)"), "https://onnxruntime.ai"},
		{"demucs-js", t("da cui viene l'elaborazione del segnale attorno alla rete, di Kevin Gibbons (MIT)", "where the signal processing around the network comes from, by Kevin Gibbons (MIT)"), "https://github.com/bakkot/demucs-js"},
		{"Ebitengine", t("la finestra e il suono, di Hajime Hoshi (Apache 2.0)", "the window and the sound, by Hajime Hoshi (Apache 2.0)"), "https://ebitengine.org"},
		{"Go", t("il linguaggio, e i caratteri Go (BSD)", "the language, and the Go fonts (BSD)"), "https://go.dev"},
		{"go-mp3, mewkiz/flac", t("leggono MP3 e FLAC", "read MP3 and FLAC"), "https://github.com/mewkiz/flac"},
	} {
		name := rect{body.x, y, 150, 28}
		if g.button(c, "credit-"+credit[0], name, credit[0], quiet) && body.has(whole.mx, whole.my) {
			show(credit[2])
		}
		for i, line := range c.wrap(credit[1], 14, regular, body.w-166) {
			c.label(line, body.x+166, y+14+float32(i)*21, 14, regular, colDim, left)
			if i > 0 {
				y += 21
			}
		}
		y += 36
	}
	y += 20
	return y - start
}
