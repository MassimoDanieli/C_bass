package app

import (
	"fmt"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// homeScreen is where a recording is chosen: a new one, or one already worked out.
func (g *Game) homeScreen(c *canvas) {
	if c.in != nil && (g.pressed(ebiten.KeyO) && (ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl))) {
		g.askForFile()
	}
	width := min(g.w-80, 760)
	x := (g.w - width) / 2
	c.label("C_bass", x, 62, 34, bold, colText, left)
	c.label(g.t("Dal brano alla parte di basso", "From a recording to a bass part"), x+2, 96, 15, regular, colDim, left)
	if g.button(c, "language", rect{x + width - 54, 48, 54, 30}, g.t("EN", "IT"), quiet) {
		g.switchLanguage()
	}

	drop := rect{x, 128, width, 170}
	c.round(drop, 16, colPanel)
	c.outline(drop.inset(0.5), 16, 1, colLine)
	c.label(g.t("Trascina qui un brano", "Drop a recording here"), g.w/2, drop.y+52, 22, medium, colText, centre)
	c.label(g.t("MP3, WAV, FLAC o M4A. La prima volta serve circa un minuto: poi si riapre subito.", "MP3, WAV, FLAC or M4A. The first time takes about a minute; after that it opens at once."), g.w/2, drop.y+84, 14, regular, colDim, centre)
	if g.button(c, "choose", rect{g.w/2 - 90, drop.y + 110, 180, 38}, g.t("Scegli un file…", "Choose a file…"), primary) {
		g.askForFile()
	}

	top := drop.y + drop.h + 34
	c.label(g.t("I brani", "Recordings"), x, top, 13, medium, colDim, left)
	list := rect{x, top + 18, width, g.h - top - 18 - 46}
	const row = 58
	if len(g.entries) == 0 {
		c.label(g.t("Ancora nessuno: quelli che apri restano qui.", "None yet: the ones you open stay here."), x, list.y+26, 14, regular, colFaint, left)
	}
	room := float32(len(g.entries))*row - list.h
	if c.in != nil && list.has(c.in.x, c.in.y) {
		g.scroll -= c.in.wheel * 30
	}
	g.scroll = max(0, min(g.scroll, max(0, room)))
	inside := c.clip(list)
	for i, entry := range g.entries {
		r := rect{x, list.y + float32(i)*row - g.scroll, width, row - 8}
		if r.y+r.h < list.y || r.y > list.y+list.h {
			continue
		}
		over := list.has(c.mx, c.my) && r.has(c.mx, c.my)
		remove := rect{r.x + r.w - 46, r.y + 9, 34, 32}
		asking := g.confirm == entry.ID
		if asking {
			remove = rect{r.x + r.w - 112, r.y + 9, 100, 32}
		}
		if inside.painting() {
			back := colPanel
			if over {
				back = colRaised
			}
			inside.round(r, 10, back)
			title := g.titleOf(entry.Title, entry.Key)
			inside.label(inside.fit(title, 16, medium, r.w-300), r.x+18, r.y+r.h/2, 16, medium, colText, left)
			if entry.BuiltIn {
				tag := g.t("incluso", "built in")
				tx := r.x + 18 + inside.width(inside.fit(title, 16, medium, r.w-300), 16, medium) + 12
				inside.round(rect{tx, r.y + r.h/2 - 9, inside.width(tag, 11, medium) + 12, 18}, 9, fade(colAccent2, 0.18))
				inside.label(tag, tx+6, r.y+r.h/2, 11, medium, colAccent2, left)
			}
			about := fmt.Sprintf("%s  ·  %.0f BPM  ·  %d %s", clock(entry.Duration), entry.Tempo, entry.Notes, g.t("note", "notes"))
			if !asking {
				inside.label(about, remove.x-14, r.y+r.h/2, 13, regular, colDim, right)
			}
		}
		inList := c.in != nil && list.has(c.in.x, c.in.y)
		if asking {
			if g.button(inside, "remove-yes-"+entry.ID, remove, g.t("Elimina", "Remove"), danger) && inList {
				if err := g.lib.Remove(entry.ID); err != nil {
					g.say(err.Error())
				}
				g.confirm = ""
				g.entries = g.lib.List()
				return
			}
		} else if over || c.in != nil {
			if g.button(inside, "remove-"+entry.ID, remove, "×", quiet) && inList {
				g.confirm = entry.ID
				continue
			}
		}
		if g.hit(c, "open-"+entry.ID, rect{r.x, r.y, r.w - (r.x + r.w - remove.x) - 6, r.h}) && inList {
			g.openEntry(entry.ID)
			return
		}
	}
	if c.in != nil && c.in.pressed && g.confirm != "" && g.active != "remove-yes-"+g.confirm {
		g.confirm = ""
	}
	c.label("C_bass "+g.version, x, g.h-24, 12, regular, colFaint, left)
	c.label(g.t("I brani analizzati restano su questo computer.", "Analysed recordings stay on this computer."), x+width, g.h-24, 12, regular, colFaint, right)
}

// workingScreen shows a recording being worked out.
func (g *Game) workingScreen(c *canvas) {
	j := g.job
	if j == nil {
		return
	}
	at, detail, done, total, downloaded, err := j.snapshot()
	width := min(g.w-80, 620)
	x := (g.w - width) / 2
	y := g.h/2 - 170
	c.label(c.fit(j.title, 24, bold, width), x, y, 24, bold, colText, left)
	if err != nil {
		message := err.Error()
		headline := g.t("Non ci sono riuscito.", "That did not work.")
		switch {
		case strings.Contains(message, "downloading"):
			headline = g.t("Non riesco a scaricare il modello: controlla la connessione e riprova.", "The model could not be downloaded: check the connection and try again.")
		case strings.Contains(message, "ffmpeg"):
			headline = g.t("Per questo tipo di file serve ffmpeg: con MP3, WAV e FLAC non serve altro.", "This kind of file needs ffmpeg: MP3, WAV and FLAC need nothing else.")
		case strings.Contains(message, "can be read"), strings.HasPrefix(message, "mp3:"), strings.HasPrefix(message, "wav:"), strings.HasPrefix(message, "flac:"):
			headline = g.t("Non riesco a leggere questo file audio.", "This audio file cannot be read.")
		case strings.Contains(message, "no notes were found"):
			headline = g.t("In questo brano non ho trovato note di basso.", "No bass notes were found in this recording.")
		}
		c.label(c.fit(headline, 16, medium, width), x, y+44, 16, medium, colDanger, left)
		for line := 0; line < 4 && message != ""; line++ {
			cut := len([]rune(message))
			for cut > 1 && c.width(string([]rune(message)[:cut]), 13, regular) > width {
				cut--
			}
			c.label(string([]rune(message)[:cut]), x, y+76+float32(line)*20, 13, regular, colFaint, left)
			message = string([]rune(message)[cut:])
		}
		if g.button(c, "back", rect{x, y + 180, 140, 38}, g.t("Indietro", "Back"), plain) || (c.in != nil && g.pressed(ebiten.KeyEscape)) {
			g.goHome()
			return
		}
		// what happened is written down: it can be looked at, and sent along with a report
		if path := diaryPath(); path != "" {
			if g.button(c, "diary", rect{x + 150, y + 180, 170, 38}, g.t("Apri il registro", "Open the log"), quiet) {
				show(path)
			}
			if g.button(c, "report", rect{x + 330, y + 180, 200, 38}, g.t("Segnala il problema", "Report the problem"), quiet) {
				show(issues)
			}
			c.label(g.t("Il registro resta su questo computer: lo mandi tu, se vuoi.", "The log stays on this computer: you send it, if you want to."), x, y+240, 12, regular, colFaint, left)
		}
		return
	}
	if at == stageLoading {
		c.label(g.t("Apro il brano…", "Opening the recording…"), x, y+44, 16, regular, colDim, left)
		return
	}
	type line struct {
		at   stage
		text string
		show bool
	}
	lines := []line{
		{stageReading, g.t("Leggo il brano", "Reading the recording"), true},
		{stageDownloading, g.t("Scarico il modello (solo la prima volta)", "Downloading the model (first time only)"), downloaded},
		{stageSeparating, g.t("Separo il basso dal resto", "Separating the bass from the rest"), true},
		{stageNotes, g.t("Leggo le note", "Reading the notes"), true},
		{stageBeat, g.t("Trovo il tempo e le battute", "Finding the tempo and the bars"), true},
		{stageChords, g.t("Leggo gli accordi", "Reading the chords"), true},
		{stageSaving, g.t("Metto via il risultato", "Putting the result away"), true},
	}
	row := y + 50
	for _, l := range lines {
		if !l.show {
			continue
		}
		ink, mark := colFaint, colLine
		switch {
		case l.at < at:
			ink, mark = colDim, colAccent
		case l.at == at:
			ink, mark = colText, colAccent
		}
		if l.at < at {
			c.disc(x+8, row, 5, mark)
		} else {
			c.ring(x+8, row, 5, 1.5, mark)
		}
		c.label(l.text, x+28, row, 16, regular, ink, left)
		if l.at == at && total > 0 {
			fraction := float32(done) / float32(total)
			note := fmt.Sprintf("%d%%", int(fraction*100))
			if at == stageDownloading {
				note = fmt.Sprintf("%s: %d / %d MB", detail, done>>20, total>>20)
			}
			c.label(note, x+width, row, 13, regular, colDim, right)
			bar := rect{x + 28, row + 17, width - 28, 4}
			c.round(bar, 2, colLine)
			c.round(rect{bar.x, bar.y, bar.w * min(1, fraction), bar.h}, 2, colAccent)
			row += 12
		}
		row += 36
	}
	c.label(g.t("Puoi lasciare la finestra aperta e fare altro.", "You can leave the window open and do something else."), x, row+16, 13, regular, colFaint, left)
	if g.button(c, "give-up", rect{x, row + 44, 140, 38}, g.t("Annulla", "Cancel"), plain) || (c.in != nil && g.pressed(ebiten.KeyEscape)) {
		g.goHome()
	}
}
