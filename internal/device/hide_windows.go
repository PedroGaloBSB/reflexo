//go:build windows

package device

import "syscall"

// createNoWindow is the CREATE_NO_WINDOW process flag. It suppresses the
// console window that would otherwise flash for every adb call, which matters
// because Reflexo polls "adb devices" in a loop.
const createNoWindow = 0x08000000

// hideConsole returns the platform settings that keep adb from opening its own
// console window.
func hideConsole() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
