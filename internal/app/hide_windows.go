//go:build windows

package app

import "syscall"

const createNoWindow = 0x08000000

// hideConsole returns the platform settings that keep a child process from
// opening its own console window. Reflexo is a windowless app, so a console
// spawned on scrcpy's behalf is a black flash that never belongs there — the
// exact per-process copy of ADR-0009's incident.
func hideConsole() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
