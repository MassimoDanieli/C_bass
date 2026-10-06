package audio

import (
	"os/exec"
	"syscall"
)

// hidden keeps a helper program from flashing a console window.
func hidden(command *exec.Cmd) *exec.Cmd {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command
}
