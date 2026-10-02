//go:build !windows

package device

import "syscall"

// hideConsole returns the platform settings that keep adb from opening its own
// console window. Only Windows has the problem, so elsewhere this is nil and
// the exec package default applies.
func hideConsole() *syscall.SysProcAttr {
	return nil
}
