package app

import "github.com/MassimoDanieli/c_bass/internal/project"

// sectionName is what a section is called in the language of the window.
func (g *Game) sectionName(kind string) string {
	switch kind {
	case "intro":
		return g.t("Intro", "Intro")
	case "verse":
		return g.t("Strofa", "Verse")
	case "prechorus":
		return g.t("Pre-ritornello", "Pre-chorus")
	case "chorus":
		return g.t("Ritornello", "Chorus")
	case "bridge":
		return g.t("Ponte", "Bridge")
	case "solo":
		return g.t("Solo", "Solo")
	case "outro":
		return g.t("Finale", "Outro")
	}
	return kind
}

// named returns the sections with their names written out, for a part going on paper.
func (g *Game) named(sections []project.Section) []project.Section {
	out := make([]project.Section, len(sections))
	for i, section := range sections {
		section.Name = g.sectionName(section.Kind)
		out[i] = section
	}
	return out
}
