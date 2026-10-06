// Command icon writes the program's icon as the set of pictures macOS makes an .icns from,
// or as a single PNG.
//
//	go run ./tools/icon C_bass.iconset     then: iconutil -c icns C_bass.iconset
//	go run ./tools/icon icon.png
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/MassimoDanieli/c_bass/internal/icon"
)

func write(path string, size int, margin float64) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, icon.Draw(size, margin))
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icon <name.iconset | name.png>")
		os.Exit(2)
	}
	target := os.Args[1]
	var err error
	if strings.HasSuffix(target, ".png") {
		err = write(target, 512, 0)
	} else if err = os.MkdirAll(target, 0o755); err == nil {
		for _, size := range []int{16, 32, 128, 256, 512} {
			if err = write(filepath.Join(target, fmt.Sprintf("icon_%dx%d.png", size, size)), size, 0.1); err != nil {
				break
			}
			if err = write(filepath.Join(target, fmt.Sprintf("icon_%dx%d@2x.png", size, size)), size*2, 0.1); err != nil {
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}
