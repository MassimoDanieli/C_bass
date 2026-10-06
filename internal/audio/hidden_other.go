//go:build !windows

package audio

import "os/exec"

// hidden keeps a helper program from flashing a console window; only Windows would.
func hidden(command *exec.Cmd) *exec.Cmd { return command }
