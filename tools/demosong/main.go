// Command demosong puts the pieces that come with the program in the library, and prints the
// name of the first: the build opens the window on it to take a picture.
//
//	CBASS_LIBRARY=folder go run ./tools/demosong
package main

import (
	"fmt"
	"os"

	"github.com/MassimoDanieli/c_bass/internal/demo"
	"github.com/MassimoDanieli/c_bass/internal/library"
)

func main() {
	lib, err := library.Open()
	if err == nil {
		var ids []string
		if ids, err = demo.Install(lib); err == nil {
			fmt.Println(ids[0])
			return
		}
	}
	fmt.Fprintln(os.Stderr, "demosong:", err)
	os.Exit(1)
}
