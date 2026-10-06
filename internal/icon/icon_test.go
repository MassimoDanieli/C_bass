package icon

import "testing"

func TestDraw(t *testing.T) {
	img := Draw(64, 0.1)
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Error("the corner is not clear")
	}
	if _, _, _, a := img.At(32, 10).RGBA(); a != 0xffff {
		t.Error("the square is not solid")
	}
	r, g, b, _ := img.At(37, 36).RGBA() // inside the note
	if r>>8 != 0xf4 || g>>8 != 0xa9 || b>>8 != 0x3a {
		t.Errorf("the note is %x %x %x, not amber", r>>8, g>>8, b>>8)
	}
}
