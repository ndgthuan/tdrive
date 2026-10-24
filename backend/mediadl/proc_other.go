//go:build !windows

// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import "os/exec"

func hide_console(cmd *exec.Cmd) {}
