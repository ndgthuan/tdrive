//go:build !windows

package mediadl

import "os/exec"

func hide_console(cmd *exec.Cmd) {}
