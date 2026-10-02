//go:build windows

package diag

import (
	"syscall"
	"unsafe"
)

// The dialog is raised with MessageBoxW rather than by embedding a GUI
// toolkit. A dependency here would be self-defeating: the package that reports
// failures is the last code that runs, and it has to keep working on a machine
// where nothing else can be installed — which is precisely the machine Reflexo
// is built for.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procMessageBoxW   = user32.NewProc("MessageBoxW")
	procGetConsoleWin = kernel32.NewProc("GetConsoleWindow")
)

// MessageBox flags. MB_TOPMOST matters because the failure often happens while
// the user is looking at something else, and a dialog that opens behind the
// browser looks like Reflexo doing nothing at all.
const (
	mbIconError = 0x00000010
	mbOK        = 0x00000000
	mbTopMost   = 0x00040000
)

// hasConsole reports whether a console window is attached to this process.
//
// A windowless build launched from the desktop has none, so stderr is a handle
// with nothing behind it: writes fail silently and the message is lost. The
// same binary run from a shell does have one, which is how --version,
// --no-browser and any debugging remain usable.
func hasConsole() bool {
	h, _, _ := procGetConsoleWin.Call()
	return h != 0
}

func show(title, detail string) {
	text, err := syscall.UTF16PtrFromString(detail)
	if err != nil {
		return
	}
	caption, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	// MessageBoxW blocks until the user dismisses it. That is the correct
	// behaviour for a fatal error: the process has already decided to stop, and
	// the user has to acknowledge why before the window disappears.
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(caption)),
		uintptr(mbIconError|mbOK|mbTopMost),
	)
}
