// Package guide turns the raw adb connection state into the single most useful
// instruction for the user.
//
// This is the product. Everything else in Reflexo exists to feed this package
// a correct State and a correct set of device properties.
//
// The guiding rule: never show a manual, show the next step.
//
// All user-facing text lives in package i18n (ADR-0006). This package contains
// only control flow and the arguments to those strings.
package guide

import (
	"unicode"

	"reflexo/internal/device"
	"reflexo/internal/i18n"
)

// Severity drives the colour and label of the instruction card.
type Severity int

const (
	// SeverityIdle is a neutral waiting state: nothing is wrong yet.
	SeverityIdle Severity = iota
	// SeverityAction means the user must do something on the phone.
	SeverityAction
	// SeverityWarn means something is wrong but not fatal.
	SeverityWarn
	// SeverityReady means mirroring can start.
	SeverityReady

	// severityEnd must stay last in this block.
	//
	// It exists because Go cannot enumerate constants, so nothing else would
	// notice a severity added without a catalog label — the badge would just
	// render blank in production. Because it is iota-derived, inserting a new
	// severity above it increments SeverityCount automatically, and
	// TestSeverityLabelsExist then fails on the unlabelled value.
	//
	// The one mistake it cannot catch is appending below severityEnd; keep new
	// severities above it.
	severityEnd
)

// SeverityCount is how many severities exist.
//
// Exported so tests can walk every one of them, which is the only way to prove
// none is left unlabelled.
const SeverityCount = int(severityEnd)

// Key returns the i18n catalog key that labels this severity.
//
// The browser cannot import this enum, so it ships the numbers and looks the
// label up by key. Returning the key from here — rather than from a parallel
// table in the test or the JavaScript — means the mapping lives next to the
// values it describes.
func (s Severity) Key() string {
	switch s {
	case SeverityIdle:
		return "severity.idle"
	case SeverityAction:
		return "severity.action"
	case SeverityWarn:
		return "severity.warn"
	case SeverityReady:
		return "severity.ready"
	default:
		return ""
	}
}

// Instruction is the card Reflexo renders.
type Instruction struct {
	Severity Severity
	// Headline is the one line that answers "what now?".
	Headline string
	// Detail explains why, in one short paragraph.
	Detail string
	// Steps are the concrete actions, in order.
	Steps []string
	// CanStart reports whether the Start button should be enabled.
	CanStart bool
}

// Situation is everything the guide needs to decide.
type Situation struct {
	// Devices is every device adb reported. Reflexo v1 supports exactly one
	// (AGENTS.md §6), so len drives the "several connected" message.
	Devices []device.Device
	// ADBAvailable reports whether the adb binary could be executed at all.
	// False usually means the driver could not be installed.
	ADBAvailable bool

	// ADBRecovering reports that Reflexo is restarting the adb server right
	// now, because it stopped answering.
	//
	// It is a separate state rather than a flavour of ADBAvailable because
	// the two need different words. "ADB indisponível" tells the user to give
	// up; "reiniciando a conexão" tells them to wait three seconds. Showing
	// the first one while a restart is in flight is how a transient hiccup
	// becomes a support ticket.
	ADBRecovering bool

	// Starting reports that Reflexo is still warming up — on a cold start the
	// adb daemon can take several seconds to answer, and during that window
	// presenting "ADB indisponível" or an empty "no phone" telling lands on
	// every first impression. It is the neutral wait, not a broken state.
	Starting bool
}

// Of returns the instruction for a situation, in the given language.
//
// Only the first device is considered: picking one arbitrarily would send the
// user to start mirroring a phone other than the one they are holding.
func Of(s Situation, lang i18n.Lang) Instruction {
	// Starting wins over everything: while Reflexo is still warming up there
	// is no honest verdict to render, and a confident wrong one is worse than
	// an honest wait.
	if s.Starting {
		return starting(lang)
	}

	if !s.ADBAvailable {
		// Recovery in flight beats the give-up message. The restart is
		// already happening, and saying so costs the user nothing.
		if s.ADBRecovering {
			return adbRecovering(lang)
		}
		return adbUnavailable(lang)
	}

	switch len(s.Devices) {
	case 0:
		return noDevice(lang)
	case 1:
		return forDevice(s.Devices[0], lang)
	default:
		return multipleDevices(len(s.Devices), lang)
	}
}

// steps builds an ordered step list from keys, discarding any that are missing
// so a partial catalog degrades instead of rendering blanks.
func steps(lang i18n.Lang, keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		out = append(out, i18n.T(lang, k))
	}
	return out
}

// noDevice is the very first screen a new user sees.
func noDevice(lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityIdle,
		Headline: i18n.T(lang, "guide.no_device.headline"),
		Detail:   i18n.T(lang, "guide.no_device.detail"),
		Steps: steps(lang,
			"guide.no_device.step1",
			"guide.no_device.step2",
			"guide.no_device.step3",
			"guide.no_device.step4",
			"guide.no_device.step5",
		),
	}
}

// adbUnavailable is the corporate-environment failure mode: no driver, no adb.
func adbUnavailable(lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityWarn,
		Headline: i18n.T(lang, "guide.adb_unavailable.headline"),
		Detail:   i18n.T(lang, "guide.adb_unavailable.detail"),
		Steps: steps(lang,
			"guide.adb_unavailable.step1",
			"guide.adb_unavailable.step2",
			"guide.adb_unavailable.step3",
		),
	}
}

// adbRecovering is shown while Reflexo restarts the adb server.
//
// The tone is deliberate. Nothing is wrong with the user's phone and nothing is
// wrong with their cable, and the previous behaviour — "ADB indisponível", with
// instructions about drivers — sent people off to reinstall things that were
// already installed. Here the honest thing to say is: Reflexo noticed, Reflexo
// is handling it, wait.
func adbRecovering(lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityWarn,
		Headline: i18n.T(lang, "guide.adb_recovering.headline"),
		Detail:   i18n.T(lang, "guide.adb_recovering.detail"),
		Steps: steps(lang,
			"guide.adb_recovering.step1",
		),
	}
}

// starting is the neutral waiting state on a cold launch.
//
// This is what the screen shows while the adb daemon still cannot be reached
// because it has not finished coming up — typically several seconds. It must
// read as "hold on", not as a verdict: it is not "no phone" and it is not
// "ADB is broken", and a false alarm here would teach the user to panic at
// every launch.
func starting(lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityIdle,
		Headline: i18n.T(lang, "guide.starting.headline"),
		Detail:   i18n.T(lang, "guide.starting.detail"),
		Steps: steps(lang,
			"guide.starting.step1",
			"guide.starting.step2",
		),
	}
}

// forDevice maps a single connected device onto the right instruction.
func forDevice(d device.Device, lang i18n.Lang) Instruction {
	switch d.State {
	case device.StateUnauthorized:
		return unauthorized(d, lang)
	case device.StateOffline:
		return offline(lang)
	case device.StateRecovery:
		return preAndroid(i18n.T(lang, "guide.pre_android.mode_recovery"), lang)
	case device.StateBootloader:
		return preAndroid(i18n.T(lang, "guide.pre_android.mode_bootloader"), lang)
	case device.StateReady:
		return ready(d, lang)
	default:
		return unknownState(d, lang)
	}
}

// unauthorized is the most common blocker: the phone is plugged in but the
// prompt was never accepted.
func unauthorized(d device.Device, lang i18n.Lang) Instruction {
	name := d.DisplayName()
	if name == "" {
		name = i18n.T(lang, "guide.unauthorized.generic_name")
	}

	return Instruction{
		Severity: SeverityAction,
		Headline: i18n.T(lang, "guide.unauthorized.headline"),
		Detail:   i18n.T(lang, "guide.unauthorized.detail", name),
		Steps: steps(lang,
			"guide.unauthorized.step1",
			"guide.unauthorized.step2",
			"guide.unauthorized.step3",
			"guide.unauthorized.step4",
		),
	}
}

// offline means adb saw the device but the handshake failed.
func offline(lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityWarn,
		Headline: i18n.T(lang, "guide.offline.headline"),
		Detail:   i18n.T(lang, "guide.offline.detail"),
		Steps: steps(lang,
			"guide.offline.step1",
			"guide.offline.step2",
			"guide.offline.step3",
			"guide.offline.step4",
		),
	}
}

// preAndroid covers states where the Android framework never starts.
//
// The mode name arrives already translated from the caller, because it varies
// by state and translating it inside would need the state passed twice.
func preAndroid(mode string, lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityWarn,
		Headline: i18n.T(lang, "guide.pre_android.headline", mode),
		Detail:   i18n.T(lang, "guide.pre_android.detail"),
		Steps: steps(lang,
			"guide.pre_android.step1",
			"guide.pre_android.step2",
			"guide.pre_android.step3",
		),
	}
}

// unknownState is a deliberate catch-all so a future adb state shows guidance
// instead of a blank screen.
func unknownState(d device.Device, lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityWarn,
		Headline: i18n.T(lang, "guide.unknown.headline"),
		Detail:   i18n.T(lang, "guide.unknown.detail", string(d.State)),
		Steps: steps(lang,
			"guide.unknown.step1",
			"guide.unknown.step2",
		),
	}
}

// multipleDevices covers the case of more than one phone attached. Reflexo v1
// mirrors one at a time (AGENTS.md §6).
func multipleDevices(n int, lang i18n.Lang) Instruction {
	return Instruction{
		Severity: SeverityAction,
		Headline: i18n.T(lang, "guide.multiple.headline", n),
		Detail:   i18n.T(lang, "guide.multiple.detail"),
		Steps: steps(lang,
			"guide.multiple.step1",
			"guide.multiple.step2",
		),
	}
}

// ready is the success path: device authorised, mirroring possible.
func ready(d device.Device, lang i18n.Lang) Instruction {
	name := d.DisplayName()
	if name == "" {
		name = i18n.T(lang, "guide.ready.generic_name")
	}

	// Even in the happy path the user has never used scrcpy before, so the
	// ready screen teaches the bindings that matter most. Without these,
	// right-click just looks like a misclick.
	all := steps(lang,
		"guide.ready.tip_right_click",
		"guide.ready.tip_middle_click",
		"guide.ready.tip_fullscreen",
		"guide.ready.step_start",
	)

	in := Instruction{
		Severity: SeverityReady,
		Headline: i18n.T(lang, "guide.ready.headline", titleize(name)),
		Detail:   describe(d, lang),
		CanStart: true,
		Steps:    all,
	}

	// Vendor advice is appended only when it is genuinely likely to matter.
	// Showing a Xiaomi warning on a Samsung would train users to ignore us.
	if note, ok := vendorNote(d, lang); ok {
		in.Detail += " " + note
		in.Steps = append(in.Steps, vendorSteps(d, lang)...)
	}

	return in
}

// describe renders the device identity line.
func describe(d device.Device, lang i18n.Lang) string {
	var parts []string
	if rel := d.AndroidRelease(); rel != "" {
		parts = append(parts, i18n.T(lang, "shared.android", rel))
	}
	if sdk := d.SDKInt(); sdk != "" {
		parts = append(parts, i18n.T(lang, "shared.api", sdk))
	}

	if len(parts) == 0 {
		return i18n.T(lang, "guide.ready.detail_unknown")
	}
	return i18n.T(lang, "guide.ready.detail_known", join(parts, " · "))
}

// vendorNote returns an extra warning for vendors whose USB debugging has a
// second, separate security switch that silently breaks input injection.
func vendorNote(d device.Device, lang i18n.Lang) (string, bool) {
	if d.Brand() == "xiaomi" {
		return i18n.T(lang, "guide.vendor.xiaomi.note"), true
	}
	return "", false
}

// vendorSteps are the concrete actions for the vendor-specific warning.
func vendorSteps(d device.Device, lang i18n.Lang) []string {
	if d.Brand() != "xiaomi" {
		return nil
	}
	return steps(lang,
		"guide.vendor.xiaomi.step1",
		"guide.vendor.xiaomi.step2",
		"guide.vendor.xiaomi.step3",
	)
}

// titleize upper-cases the first letter of a device name for the headline.
//
// Device model names frequently arrive already in upper case ("SM-A556B") or
// as bare codes ("Pixel 8"); those are left untouched so we do not mangle
// something the user recognises.
func titleize(s string) string {
	if s == "" || isAllUpper(s) {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// isAllUpper reports whether a string contains no lowercase letters, used to
// avoid mangling model codes.
func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return hasLetter
}

// join concatenates strings with a separator.
func join(items []string, sep string) string {
	var b []byte
	for i, s := range items {
		if i > 0 {
			b = append(b, sep...)
		}
		b = append(b, s...)
	}
	return string(b)
}
