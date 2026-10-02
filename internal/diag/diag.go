// Package diag reports failures that happen before — or instead of — the
// browser being available.
//
// Reflexo normally talks to the user through a page in their browser, which is
// the right channel: it is already open, it is in their language, and it can
// hold a sentence. But two situations leave no page to write on:
//
//   - Reflexo is built without a console window, so a double-click from the
//     desktop shows nothing at all. That is deliberate (see the ADR on the
//     windowless build) because a console window is a kill switch: closing it
//     delivers a control event that terminates the process, which is exactly
//     how a previous build kept "quitting by itself". The cost of removing that
//     failure mode is that a failure at startup has nowhere to be printed.
//   - A failure while starting up happens before there is a server, so there is
//     no page yet.
//
// This package is that missing channel. It is deliberately tiny and has no
// dependencies: a failure reporter must not be able to introduce new ones.
package diag

import (
	"fmt"
	"os"
)

// NotePath is where a failure is recorded on platforms that have no
// dependency-free way to raise a dialog: the text is written there and handed
// to the desktop so the user sees something. main points it at a file beside
// the log, so the two artifacts a user might send for help sit together.
//
// Windows ignores it — MessageBoxW covers that case — but the variable is
// declared here so that setting it compiles everywhere, rather than in the
// build-tagged files where it would be a platform-specific trap for whoever
// wires this up next.
var NotePath = ""

// Fatal reports an unrecoverable failure and is expected not to return on
// Windows, where the user must dismiss a dialog before the process ends.
//
// detail should already be written for someone who does not read English well
// and does not know what a stack trace is. The log path belongs in it: the log
// is the artifact that lets a second person help them.
func Fatal(title, detail string) {
	// stderr first. When a terminal is attached — someone running Reflexo from
	// a shell, which is how --version and --no-browser are meant to be used —
	// this is the correct and scriptable channel, and no dialog should appear.
	fmt.Fprintln(os.Stderr, detail)

	if hasConsole() {
		return
	}
	show(title, detail)
}

// hasConsole and show are provided per platform: only Windows can raise a
// native dialog without a dependency, and only the others can hand a file to
// the desktop. See diag_windows.go and diag_other.go.
