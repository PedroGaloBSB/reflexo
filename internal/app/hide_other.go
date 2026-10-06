//go:build !windows

package app

import "syscall"

// hideConsole returns the platform settings that keep a child process from
// opening its own console window. Only Windows has the problem; elsewhere this
// is nil and the exec package default applies.
func hideConsole() *syscall.SysProcAttr {
	return nil
}
