package device

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// This file exists because of a real session loss, captured in the log on
// 2026-10-05:
//
//	10:40:21  aparelho: dq7ppfnjr84tjnuw estado=device
//	10:40:23  espelhamento iniciado
//	10:46:15  espelhamento terminou com erro: exit status 2
//	10:46:20  ERROR  adb devices falhou: * daemon not running
//
// The phone never refused anything and never lost authorisation. The adb server
// process died, scrcpy lost the transport and exited 2, and from then on every
// poll failed. Reflexo reported "ADB indisponível" and then sat there, correctly
// describing a problem it could not fix and would not try to fix.
//
// The user's phone worked fine the whole time. A tool whose failure mode is
// "stops working and does not recover" is not finished.

// FailureKind says why adb could not be reached. The kind chooses the recovery,
// so guessing wrong here is expensive: killing a healthy server over a
// transient hiccup is worse than waiting.
type FailureKind int

const (
	// FailureNone means the last call worked.
	FailureNone FailureKind = iota

	// FailureDaemonDead means the server process is gone. adb says so in as
	// many words. The cheap fix is enough: ask it to start.
	FailureDaemonDead

	// FailureDaemonHung means a server exists but never answers. This is the
	// case that needs killing, because "start" on a live-but-wedged server
	// reports success and changes nothing.
	FailureDaemonHung

	// FailureForeignServer means the server on the port was started by a
	// different adb version. Two adb builds cannot share one server, and this
	// is not rare: any other tool that ever ran adb leaves one behind.
	FailureForeignServer

	// FailureNotInstalled means the adb binary is missing or not executable.
	// No amount of restarting helps.
	FailureNotInstalled

	// FailureUnknown means it failed in a way we do not recognise.
	FailureUnknown
)

func (k FailureKind) String() string {
	switch k {
	case FailureNone:
		return "nenhuma"
	case FailureDaemonDead:
		return "daemon encerrado"
	case FailureDaemonHung:
		return "daemon travado"
	case FailureForeignServer:
		return "daemon de outra versão"
	case FailureNotInstalled:
		return "adb ausente"
	default:
		return "desconhecida"
	}
}

// NeedsRestart reports whether recovery should kill the existing server.
//
// Nothing does. This method exists because the obvious implementation was
// tempting and wrong, and the reasoning is worth keeping in one place.
//
// adb is a machine-wide singleton on tcp:5037. Android Studio, VS Code, every
// vendor tool and every other adb on the machine talk to that one server.
// Killing it to fix our own symptom means breaking tools the user did not ask
// us to touch, on a product whose entire premise is not taking over the
// machine (AGENTS.md §4).
//
// What looked like a reasonable exception was the wedged-server case. Measured
// on a real machine, a wedged daemon answers "adb start-server" with
// "could not read ok from ADB Server" and a non-zero exit, because the client
// launches the daemon itself and the handshake fails. Retrying hard then
// competes with the previous attempt's half-started process and the port never
// settles — the recovery makes things worse, which is the opposite of the point.
//
// So recovery is non-destructive: ask the server to start, and let it. A
// genuinely wedged server is a condition the user can resolve, and the message
// now says so instead of looping forever.
func (k FailureKind) NeedsRestart() bool {
	return false
}

// Classify works out what went wrong from the error and whatever adb printed.
//
// adb is not consistent about this: the same condition surfaces as a non-zero
// exit, as text on stderr, or as a hang. Matching on the text is not elegant,
// but it is the only channel adb offers, and every pattern here was read off a
// real failure rather than guessed from documentation.
func Classify(err error, output string) FailureKind {
	if err == nil {
		return FailureNone
	}

	text := strings.ToLower(output + " " + err.Error())

	switch {
	case strings.Contains(text, "not recognized") ||
		strings.Contains(text, "cannot find") ||
		strings.Contains(text, "no such file") ||
		strings.Contains(text, "file does not exist") ||
		strings.Contains(text, "is not recognized as"):
		return FailureNotInstalled

	case strings.Contains(text, "daemon not running") ||
		strings.Contains(text, "cannot connect to daemon") ||
		strings.Contains(text, "failed to start daemon"):
		return FailureDaemonDead

	// The server answered but refused the connection. It is alive and belongs
	// to somebody else.
	case strings.Contains(text, "protocol fault") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "unexpected token") ||
		strings.Contains(text, "closed") && strings.Contains(text, "adb server"):
		return FailureForeignServer

	case errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(text, "timed out") ||
		strings.Contains(text, "timeout"):
		return FailureDaemonHung
	}

	// exec errors with no adb output at all are almost always a missing or
	// unusable binary.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && strings.TrimSpace(output) == "" {
		return FailureNotInstalled
	}

	return FailureUnknown
}

// Recover asks adb to bring its server back.
//
// It is deliberately non-destructive. The server on tcp:5037 is shared with
// every other adb-based tool on the machine, so this never kills it; it only
// asks for a server to be started. See FailureKind.NeedsRestart for the
// measurements behind that choice.
//
// It returns the kind it acted on and whether the follow-up list call worked,
// because "we tried to recover" and "we recovered" are different facts and the
// log must not blur them.
func (c *Client) Recover(ctx context.Context, kind FailureKind) (FailureKind, error) {
	// A restart is slower than a normal call: it waits for a process to come
	// up. Reusing the per-call timeout would be measured against the wrong
	// operation.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if _, err := c.run(ctx, "start-server"); err != nil {
		return kind, err
	}

	// A successful start does not mean a working adb. Only asking again does.
	if _, err := c.List(ctx); err != nil {
		out, _ := c.run(ctx, "devices")
		return Classify(err, out), fmt.Errorf("adb continua sem responder apos reiniciar: %w", err)
	}

	return kind, nil
}

// Healthy reports whether adb is answering right now.
//
// It exists for the case that bit hardest: "adb devices" can exit successfully
// and print an empty list while the server is on its way out. Empty then means
// nothing at all — it is the same output a machine with no phone produces. Only
// a second, independent call can tell the two apart.
func (c *Client) Healthy(ctx context.Context) bool {
	_, err := c.run(ctx, "devices")
	return err == nil
}
