package app

import "reflexo/internal/device"

// debuggingOff reports whether a phone is plugged in but is not exposing the
// ADB interface — the situation ADR-0007 was opened for, where "no phone found"
// is technically true and practically a lie.
//
// It is consulted from exactly one place: after adb has answered and reported
// nothing. That placement is the whole safety argument, and it is worth being
// explicit about why the other spots are wrong:
//
//   - While adb is failing, an empty bus means nothing. The phone may be fine
//     and the server may be dead, and the adb health machine already owns that
//     message. Accusing the user's phone here would be a guess.
//   - When a phone is already visible to adb there is nothing left to explain.
//
// The USB walk is not free but is small enough not to need a cache: measured at
// ~11ms per call with 121 present devices, and it runs only while adb sees
// nothing, which is to say while the user is idle waiting for a phone. A cache
// would have meant a second piece of state to invalidate, for a saving this
// codebase does not need.
func (a *App) debuggingOff() bool {
	probe := a.usbProbe
	if probe == nil {
		probe = device.UsbAndroidState
	}

	state := probe()

	a.mu.Lock()
	sig := state.String()
	changed := sig != a.lastUsbSig
	a.lastUsbSig = sig
	log := a.log
	a.mu.Unlock()

	if changed {
		log.Infof("usb: %s", sig)
	}
	return state == device.UsbDebuggingOff
}
