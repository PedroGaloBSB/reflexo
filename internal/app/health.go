package app

import (
	"context"
	"time"

	"reflexo/internal/device"
	"reflexo/internal/guide"
)

// This file is the response to a real session loss, not a precaution.
//
// The log from 2026-10-05 records it exactly: the phone was connected and
// authorised, the mirror had been running for six minutes, and then the adb
// server died. scrcpy lost the transport and exited 2. From that moment every
// poll failed, and Reflexo displayed "ADB unavailable" — accurate, and useless,
// because it never tried to fix anything and never would. The user's phone was
// working the entire time. Restarting Reflexo was the only fix, and nothing in
// the product said that.
//
// So: classify the failure, act on it, back off, and say what is happening.

// recoveryDelays is the backoff schedule, in seconds, indexed by attempt.
//
// It starts at 5s because the poll runs every 2s: reacting on the second
// failure is right, reacting on the first is reacting to noise. It grows to
// 60s because a server that stays broken for a minute is not coming back on its
// own, and hammering start-server thirty times a minute helps nobody and risks
// colliding with other tools that use adb on the same machine.
var recoveryDelays = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	40 * time.Second,
	60 * time.Second,
}

// failureThreshold is how many consecutive failed polls before Reflexo acts.
//
// One failure is not evidence. A phone being unplugged, a USB transaction
// retrying, or a momentary hiccup all produce a single bad poll, and killing a
// healthy adb server over one of those would break whatever else on the machine
// is using adb. Two in a row is a pattern.
const failureThreshold = 2

// startupGrace is how long after launch adb failures are expected rather than
// reported.
//
// This is not defensive coding; it is a measurement. On 2026-10-05, on real
// hardware, "adb start-server" returned after ~1.5s with "protocol fault:
// connection reset" while the daemon it had just spawned needed ~8s to actually
// listen on tcp:5037. The first poll runs 2s after launch, so *every single
// cold start* opened with the screen saying the ADB was unavailable, for eight
// seconds, on a perfectly healthy machine.
//
// That is the worst possible first impression and it happens every time, which
// makes it worse than a rare failure: users learn to ignore the message, and
// then it is useless when it is true.
//
// During the grace window a failing adb is reported as "waiting", because that
// is the truth — Reflexo is still starting, and nothing is wrong.
//
// The value is measured, not guessed. Polling "adb devices" from cold on the
// test machine, with the daemon dead, took 7.4s before it answered — the client
// itself launches the daemon, and it needs that long to listen. A 15s window was
// tried first and was still not enough: the log showed the first recovery
// attempt at 17s, which is inside what a user perceives as "it did not work".
//
// 30s is deliberate. It is long enough to cover the measured cold start with
// room for a slower machine, and short enough that a genuine failure is not
// hidden for a minute. Below roughly 20s this is measurably wrong on the
// hardware it was measured on.
const startupGrace = 30 * time.Second

// checkADB decides what the current poll should conclude, recovering the server
// when it is warranted.
//
// It returns the situation to render. Keeping this separate from Refresh is
// what makes it testable: the state machine has no clock of its own and no
// direct dependency on adb, so the whole recovery path can be driven
// deterministically.
func (a *App) checkADB(ctx context.Context, devices []device.Device, listErr error) guide.Situation {
	// The happy path, and the overwhelmingly common one.
	if listErr == nil {
		// An empty list right after the mirror died is not the same as an
		// empty list on an idle desk. "adb devices" can exit successfully
		// while printing nothing because the server is on its way out, and
		// that output is byte-for-byte what a machine with no phone produces.
		// Without this check, losing the phone and losing adb are
		// indistinguishable, which is the original bug in a new place.
		if len(devices) == 0 && a.suspiciousEmpty() {
			if a.probeAndRecover(ctx) {
				return guide.Situation{Devices: devices, ADBAvailable: true}
			}
		}

		a.adbHealthy()
		// DebuggingOff is the one case adb cannot express. Its empty list is
		// truthful in both "nothing plugged in" and "plugged in with debugging
		// switched off", and only the USB bus can tell those apart.
		return guide.Situation{
			Devices:      devices,
			ADBAvailable: true,
			DebuggingOff: len(devices) == 0 && a.debuggingOff(),
		}
	}

	kind := device.Classify(listErr, "")
	a.noteFailure(kind)

	// Startup grace. The daemon needs several seconds to listen on 5037 and the
	// first poll lands inside that window, so failing there is the normal cold
	// start, not a fault. Reporting it as one would put "ADB indisponível" on
	// screen every single launch.
	if a.withinStartupGrace() {
		return guide.Situation{Starting: true}
	}

	// Not enough evidence yet. Report the failure honestly and wait one more
	// poll; the state is still the one that says the truth, not "no phone".
	if a.adbFailures < failureThreshold {
		return guide.Situation{ADBRecovering: a.recovering()}
	}

	if !a.recoveryDue() {
		return guide.Situation{ADBRecovering: a.recovering()}
	}

	return a.attemptRecovery(ctx, kind)
}

// withinStartupGrace reports whether Reflexo is still in its launch window.
func (a *App) withinStartupGrace() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return time.Since(a.startedAt) < startupGrace
}

// adbHealthy resets the failure bookkeeping.
func (a *App) adbHealthy() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.adbRecovering {
		a.log.Infof("adb voltou a responder (tentativas de reinicio: %d)", a.recoveryTried)
	}
	a.adbFailures = 0
	a.adbRecovering = false
	a.recoveryTried = 0
	a.adbKind = device.FailureNone
}

// noteFailure records a failed poll.
func (a *App) noteFailure(kind device.FailureKind) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.adbFailures++
	if kind != device.FailureUnknown {
		a.adbKind = kind
	}
}

func (a *App) recovering() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.adbRecovering
}

// recoveryDue reports whether enough time has passed since the last attempt.
func (a *App) recoveryDue() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.recoveryTried == 0 {
		return true
	}
	idx := a.recoveryTried - 1
	if idx >= len(recoveryDelays) {
		idx = len(recoveryDelays) - 1
	}
	return time.Since(a.lastRecovery) >= recoveryDelays[idx]
}

// attemptRecovery restarts the adb server and reports what happened.
//
// Two rules learned the hard way while testing this on a real machine, both
// worth more than the state machine around them.
//
// First, a dead server is not revived by asking twice. When the daemon is
// wedged, "adb start-server" returns "could not read ok from ADB Server" and
// exits non-zero, because the client itself launches the daemon and the
// handshake fails. Retrying in a tight loop then competes with the previous
// attempt's own half-started process and keeps the port from ever settling.
//
// Second, and more important: killing the server is not free. adb is a
// machine-wide singleton on port 5037, shared with Android Studio, VS Code,
// any vendor tool and every other adb on the machine. Tearing that down to fix
// our own symptom would break other people's tools — which is a far worse
// outcome than showing a message and asking the user what to do.
func (a *App) attemptRecovery(ctx context.Context, kind device.FailureKind) guide.Situation {
	// No adb client means no recovery is possible: the binary was never
	// installed, or the install is still running. Returning the plain
	// unavailable state is the honest answer.
	//
	// This is also a nil-pointer guard, not just a logic check. attemptRecovery
	// runs on the poll path, and a panic there would take down the whole
	// process rather than degrade the screen — the test for it is
	// TestRecoveryWithoutADBClientDoesNotPanic.
	if a.adb == nil {
		return guide.Situation{ADBRecovering: true}
	}

	a.mu.Lock()
	a.adbRecovering = true
	a.recoveryTried++
	a.lastRecovery = time.Now()
	attempt := a.recoveryTried
	actOn := a.adbKind
	a.mu.Unlock()

	if actOn == device.FailureNone {
		actOn = kind
	}

	log := a.logger()
	log.Warnf("adb nao responde (%s), reiniciando (tentativa %d)", actOn, attempt)

	// Recovery is the one thing that must not be cancelled by the poll
	// context: it waits for a process to come up, and a poll timeout would
	// abort it halfway and leave the next attempt racing this one.
	_, err := a.adb.Recover(context.WithoutCancel(ctx), actOn)

	// The attempt has happened. Whether the next poll sees a working adb is a
	// separate question that checkADB answers on its own, so the failure
	// counter is reset here rather than left accumulating across attempts and
	// tripping the threshold again before the server had a chance to come up.
	a.mu.Lock()
	a.adbFailures = 0
	a.adbKind = device.FailureNone
	a.adbRecovering = false
	a.state.ADBRecovering = false
	a.mu.Unlock()

	if err != nil {
		log.Warnf("reinicio do adb falhou: %v", err)
		// The attempt failed, so the user needs the dead-end message, not the
		// reassuring one. Claiming a recovery that did not happen is the
		// dishonesty this whole file exists to remove.
		return guide.Situation{ADBRecovering: false}
	}

	log.Infof("adb reiniciado com sucesso")

	// The restart worked. Say so, and let the next poll confirm by listing the
	// devices normally.
	return guide.Situation{ADBRecovering: true}
}

// suspiciousEmpty reports whether an empty device list follows so soon after
// the mirror died that adb is the more likely explanation.
//
// A phone that was mirroring twenty seconds ago and is now invisible is a phone
// that was unplugged. A phone that was mirroring and whose server vanished is
// the case that cost a support session. They look identical from the outside,
// and the only cheap way to tell them apart is to ask adb a second time.
func (a *App) suspiciousEmpty() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return !a.mirrorLostAt.IsZero() && time.Since(a.mirrorLostAt) < 30*time.Second
}

// probeAndRecover confirms adb is really alive, and restarts it if not.
func (a *App) probeAndRecover(ctx context.Context) bool {
	if a.adb == nil {
		return false
	}
	if a.adb.Healthy(ctx) {
		return false
	}

	a.logger().Warnf("lista de aparelhos vazia logo apos a queda do espelho: " +
		"o adb pode ter parado de responder")

	a.mu.Lock()
	a.adbFailures = failureThreshold // a real failure from here on
	a.mu.Unlock()

	a.attemptRecovery(context.WithoutCancel(ctx), device.FailureDaemonDead)
	return true
}

// noteMirrorLost records when the mirror process exited, so an empty list right
// afterwards can be treated as suspicious.
func (a *App) noteMirrorLost() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mirrorLostAt = time.Now()
}
