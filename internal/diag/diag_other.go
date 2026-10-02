//go:build !windows

package diag

import (
	"os"
	"os/exec"
	"runtime"
)

// hasConsole reports whether stderr is a terminal.
//
// The test is whether stderr is a character device: a real terminal is, while
// a file or a pipe is not. It misreads /dev/null as a terminal, which costs
// nothing, because a run whose stderr was sent to /dev/null has deliberately
// asked not to be talked to.
func hasConsole() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func show(title, detail string) {
	// Linux and macOS have no equivalent of MessageBox in the standard
	// library, and pulling in a GUI toolkit to print one sentence would add
	// more risk than the sentence removes. Instead the failure is written where
	// it can be found and handed to whatever the system uses to open text
	// files. When that is unavailable the file itself is still the record, and
	// the log has the same content.
	body := title + "\n\n" + detail + "\n"

	if NotePath != "" {
		_ = os.WriteFile(NotePath, []byte(body), 0o644)
		open(NotePath)
		return
	}

	os.Stdout.WriteString(body)
}

// open asks the desktop to display path with the user's default handler.
func open(path string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return
	}

	// Detached and with output discarded: this is a courtesy, and a failure to
	// show a window must not become a second error on top of the first.
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
	go func() { _ = cmd.Wait() }()
}
