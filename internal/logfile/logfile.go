// Package logfile writes Reflexo's run log.
//
// Why a file and not just the console: the console disappears when the process
// ends, which is exactly when a problem needs explaining. Reflexo is aimed at
// people who cannot interpret a stack trace, so the log has two audiences —
// the user reporting "it didn't work", and whoever helps them afterwards. Both
// read the same plain-text file.
//
// Two rules shape everything here:
//
//   - A logging call must never become the failure. Every method is safe on a
//     nil *Logger, ignores write errors after a few retries, and never panics.
//     A full disk or a read-only profile must not stop the mirroring.
//   - The log lives in the user's LOCAL application data, never in Roaming.
//     Roaming is frequently redirected to OneDrive in corporate environments —
//     our primary audience — and the log contains device serials, which have
//     no business reaching a cloud sync. It also contains a device serial that
//     the user may not want written anywhere shared.
//
// Size is capped and rotated, so a long-lived install cannot fill a profile
// disk: see maxBytes and keepBackups.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Rotation limits. Three files of 2 MB is 8 MB worst case, which is small
// enough to ignore and large enough to cover a long troubleshooting session.
const (
	maxBytes    = 2 << 20 // 2 MB per file
	keepBackups = 3
)

// Level is the severity of one line. It is written as text so the file reads
// sensibly when a user opens it in Notepad.
type Level string

// The levels, in order of severity.
const (
	Info  Level = "INFO"
	Warn  Level = "WARN"
	Error Level = "ERROR"
)

// Logger appends timestamped lines to a file.
//
// The zero value and a nil *Logger are both usable and do nothing, so callers
// never need a nil check and the product still runs if logging cannot start.
type Logger struct {
	mu   sync.Mutex
	f    *os.File
	path string

	// failed counts consecutive write errors, and disabled latches the logger
	// off once it gives up, rather than retrying on every 2-second poll.
	failed   int
	disabled bool

	// closed marks a finished logger. It is separate from f being nil because
	// reopening is normal during rotation, but reopening after Close would
	// resurrect the file — leaving a handle open after shutdown, and letting a
	// straggler goroutine append to a log we already reported as finished.
	closed bool
}

// Open creates (or reopens) the log file inside dir.
//
// It rotates an oversized file on the way in, so a crashed previous run never
// leaves an append that grows without bound.
func Open(dir, name string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("não foi possível criar a pasta de log em %s: %w", dir, err)
	}

	l := &Logger{path: filepath.Join(dir, name)}

	// Rotate before opening. The file is closed right now, so the rename can
	// succeed — Windows refuses to rename a file that is open.
	if err := rotateIfOversized(l.path); err != nil {
		return nil, fmt.Errorf("não foi possível rotacionar o log em %s: %w", l.path, err)
	}

	if err := l.openLocked(); err != nil {
		return nil, err
	}
	return l, nil
}

// openLocked opens the file for appending. Caller holds l.mu.
func (l *Logger) openLocked() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("não foi possível abrir o log em %s: %w", l.path, err)
	}
	l.f = f
	l.failed = 0
	return nil
}

// closeLocked closes the file if open. Caller holds l.mu.
func (l *Logger) closeLocked() error {
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Path returns the log file location, for telling the user where to find it.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Close flushes and closes the file. Safe on a nil Logger, and idempotent.
//
// It is final: writes after Close are discarded rather than reopening the file.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.closed = true
	return l.closeLocked()
}

// Infof records a normal event.
func (l *Logger) Infof(format string, args ...any) { l.logf(Info, format, args...) }

// Warnf records something suspicious that did not stop Reflexo.
func (l *Logger) Warnf(format string, args ...any) { l.logf(Warn, format, args...) }

// Errorf records a failure the user may notice.
func (l *Logger) Errorf(format string, args ...any) { l.logf(Error, format, args...) }

// maxConsecutiveFailures is deliberately low. If the disk is full we want to
// stop trying quickly, not emit one failed write per 2-second poll.
const maxConsecutiveFailures = 5

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil {
		return
	}

	// Sprintf first, outside the lock: formatting is the expensive part and
	// there is no reason to serialise it.
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}

	line := fmt.Sprintf("%s  %-5s  %s\n",
		time.Now().Format("2006-01-02 15:04:05.000"), level, flatten(msg))

	// A newline inside a message would break the one-event-per-line contract
	// that makes the file greppable.
	l.write([]byte(line))
}

func flatten(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func (l *Logger) write(b []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.disabled || l.closed {
		return
	}

	if err := l.reopenIfNeeded(int64(len(b))); err != nil {
		l.failed++
		if l.failed >= maxConsecutiveFailures {
			l.disabled = true
			_ = l.closeLocked()
		}
		return
	}

	if _, err := l.f.Write(b); err != nil {
		l.failed++
		if l.failed >= maxConsecutiveFailures {
			l.disabled = true
			_ = l.closeLocked()
		}
	}
}

// reopenIfNeeded opens the log if it is closed, and rotates it first when the
// incoming line would push it past the size limit.
//
// The rotation deliberately happens with the file closed. Windows refuses to
// rename a file that is open, so rotating in place silently failed there: the
// write was skipped, five failures later the logger latched off, and the log
// went quiet precisely once it had grown large enough to be worth reading.
func (l *Logger) reopenIfNeeded(incoming int64) error {
	if l.f == nil {
		return l.openLocked()
	}

	if size, err := fileSize(l.path); err != nil {
		// The file vanished under us (deleted by a cleaner, for example).
		// Reopen rather than keep writing to an unlinked handle.
		_ = l.closeLocked()
		return l.openLocked()
	} else if size+incoming <= maxBytes {
		return nil
	}

	if err := l.closeLocked(); err != nil {
		return err
	}
	if err := rotateIfOversized(l.path); err != nil {
		return err
	}
	return l.openLocked()
}

// fileSize returns the size of path, or an error if it does not exist.
func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// rotateIfOversized renames path.1..keepBackups-1 and starts a fresh path.
//
// Rotation before writing rather than after keeps a single line from ever being
// split across two files, which would make the tail of the file unreadable.
func rotateIfOversized(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Size() < maxBytes {
		return nil
	}

	oldest := fmt.Sprintf("%s.%d", path, keepBackups)
	_ = os.Remove(oldest)

	for i := keepBackups - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, fmt.Sprintf("%s.%d", path, i+1)); err != nil {
			return err
		}
	}

	if err := os.Rename(path, path+".1"); err != nil {
		return err
	}
	return nil
}

// Recover logs a panic with its stack and returns the value to repanic with.
//
// A panic in a goroutine — which is where all of Reflexo's work happens — is
// otherwise invisible: the process dies and the console that would have shown
// the trace is already gone. With the log, the trace survives.
func (l *Logger) Recover(what string) {
	if r := recover(); r != nil {
		buf := make([]byte, 16<<10)
		n := runtime.Stack(buf, false)
		l.Errorf("panic em %s: %v\n%s", what, r, buf[:n])
		panic(r)
	}
}

// Dir returns the directory Reflexo logs into.
//
// os.UserCacheDir is LOCALAPPDATA on Windows, ~/.cache on Linux and
// ~/Library/Caches on macOS. The Windows mapping is the important one: it is
// outside Roaming, so it is never synced by OneDrive or a domain profile.
func Dir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		// Last resort. Never block the product over a log location.
		return filepath.Join(os.TempDir(), "reflexo-logs"), nil
	}
	return filepath.Join(base, "reflexo", "logs"), nil
}
