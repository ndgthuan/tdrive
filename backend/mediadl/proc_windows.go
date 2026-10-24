// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW keeps a console window from flashing for each tool run in
// the GUI process.
const CREATE_NO_WINDOW = 0x08000000

func hide_console(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: CREATE_NO_WINDOW}
}
