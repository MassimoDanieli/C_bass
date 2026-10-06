// Command cbass-app is C_bass with its window: drop a recording on it, and play along with
// the tablature and the neck.
package main

import (
	"fmt"
	"os"

	"github.com/MassimoDanieli/c_bass/internal/app"
)

// version is set at build time.
var version = "dev"

func main() {
	if err := app.Run(version); err != nil {
		fmt.Fprintln(os.Stderr, "C_bass:", err)
		os.Exit(1)
	}
}
