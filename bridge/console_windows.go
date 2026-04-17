//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableVT() {
	for _, fd := range []uintptr{os.Stdout.Fd(), os.Stderr.Fd()} {
		h := windows.Handle(fd)
		var mode uint32
		if err := windows.GetConsoleMode(h, &mode); err != nil {
			continue
		}
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	_ = windows.SetConsoleOutputCP(65001)
}
