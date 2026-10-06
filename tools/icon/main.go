// Command icon writes the program's icon: as the set of pictures macOS makes an .icns from,
// as a Windows .ico, or as a single PNG.
//
//	go run ./tools/icon C_bass.iconset     then: iconutil -c icns C_bass.iconset
//	go run ./tools/icon C_bass.ico
//	go run ./tools/icon icon.png
package main

import (
	"bytes"
	"encoding/binary"
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

// ico packs the icon at several sizes into a Windows icon file. The small sizes are stored as
// plain bitmaps, which every part of Windows reads; the large one as a PNG, as is usual.
func ico(sizes ...int) []byte {
	var images [][]byte
	for _, size := range sizes {
		img := icon.Draw(size, 0)
		var data bytes.Buffer
		if size >= 256 {
			png.Encode(&data, img)
		} else {
			// a bitmap header, the pixels from the bottom row up as blue, green, red and
			// alpha, then the old one-bit mask, left empty since the alpha says it all
			header := make([]byte, 40)
			binary.LittleEndian.PutUint32(header[0:], 40)
			binary.LittleEndian.PutUint32(header[4:], uint32(size))
			binary.LittleEndian.PutUint32(header[8:], uint32(size*2))
			binary.LittleEndian.PutUint16(header[12:], 1)
			binary.LittleEndian.PutUint16(header[14:], 32)
			data.Write(header)
			for y := size - 1; y >= 0; y-- {
				for x := 0; x < size; x++ {
					c := img.RGBAAt(x, y) // premultiplied: undo that, a bitmap wants plain colours
					r, g, b := c.R, c.G, c.B
					if c.A > 0 && c.A < 255 {
						r, g, b = uint8(int(c.R)*255/int(c.A)), uint8(int(c.G)*255/int(c.A)), uint8(int(c.B)*255/int(c.A))
					}
					data.Write([]byte{b, g, r, c.A})
				}
			}
			data.Write(make([]byte, size*((size+31)/32*4)))
		}
		images = append(images, data.Bytes())
	}
	out := make([]byte, 6+16*len(images))
	binary.LittleEndian.PutUint16(out[2:], 1)
	binary.LittleEndian.PutUint16(out[4:], uint16(len(images)))
	offset := len(out)
	for i, data := range images {
		entry := out[6+16*i:]
		entry[0], entry[1] = byte(sizes[i]), byte(sizes[i]) // 256 is written as 0, which a byte does by itself
		binary.LittleEndian.PutUint16(entry[4:], 1)
		binary.LittleEndian.PutUint16(entry[6:], 32)
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(data)))
		binary.LittleEndian.PutUint32(entry[12:], uint32(offset))
		offset += len(data)
	}
	for _, data := range images {
		out = append(out, data...)
	}
	return out
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icon <name.iconset | name.ico | name.png>")
		os.Exit(2)
	}
	target := os.Args[1]
	var err error
	if strings.HasSuffix(target, ".png") {
		err = write(target, 512, 0)
	} else if strings.HasSuffix(target, ".ico") {
		err = os.WriteFile(target, ico(16, 24, 32, 48, 64, 256), 0o644)
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
