package device

import "strings"

// UsbState is what can be concluded about a phone on the USB bus, independent
// of what adb reports.
//
// This exists because of a specific silence in the product. "adb devices"
// returns an empty list in two completely different situations:
//
//  1. No phone is plugged in at all.
//  2. A phone IS plugged in, with USB debugging switched off.
//
// In the second case the phone behaves like a thumb drive: it enumerates as MTP
// and simply never advertises the ADB interface. That was measured on a real
// SM-A556E, where PnP showed MI_00 (MTP) and MI_01 (serial) present while
// MI_03 (ADB Interface) was absent.
//
// Telling the user "no phone found" there is worse than useless — the phone is
// physically in front of them. It is the one situation where the product makes
// the user doubt reality, and it is the failure case a beginner hits most.
type UsbState int

const (
	// UsbUnknown means the platform gives us no way to tell, so nothing should
	// be claimed. This is the honest default: a missing implementation must
	// never turn into a wrong message.
	UsbUnknown UsbState = iota

	// UsbNoDevice means no Android phone is present on the USB bus.
	UsbNoDevice

	// UsbDebuggingOff means a phone is present but is not exposing the ADB
	// interface. The cable is fine, the phone is fine; only the debugging
	// setting is wrong.
	UsbDebuggingOff
)

// String names the state for the log file. A support bundle is read by a
// person, not by a renderer.
func (s UsbState) String() string {
	switch s {
	case UsbNoDevice:
		return "nenhum"
	case UsbDebuggingOff:
		return "depuração desligada"
	default:
		return "desconhecido"
	}
}

// usbSignals is what a raw walk of the USB bus observed, before any judgement.
//
// Keeping the observation separate from the conclusion is what makes the
// conclusion testable: the walk is Windows-only and depends on whatever is
// plugged in, while usbStateFrom is pure and can be exercised on any platform
// against any combination of signals, including the ones this machine cannot
// currently produce.
type usbSignals struct {
	// failed means the bus could not be enumerated at all. It is tracked
	// rather than folded into "nothing found", because those two answers lead
	// to opposite conclusions and confusing them is precisely the mistake this
	// file was written to prevent.
	failed bool

	// adbPresent means some interface advertises the Android debug function.
	adbPresent bool

	// mtpAndroid means some interface advertises file transfer and belongs to
	// a vendor Reflexo recognises as a phone manufacturer.
	mtpAndroid bool

	// mtpForeign is "vid:pid" for every file-transfer interface whose vendor is
	// not on the list. It is carried so the log can name the device that was
	// not recognised, which is how androidVendors grows from evidence instead of
	// from a guess.
	mtpForeign []string

	// scanned counts the present devices actually read. A zero with failed
	// false means the walk ran and the bus genuinely holds nothing interesting.
	scanned int
}

// usbStateFrom turns observations into the one conclusion Reflexo is allowed to
// draw.
//
// The order of the cases is the decision, and each one is a guard against
// accusing the user of something untrue:
//
//	failed      -> say nothing. No knowledge is not evidence of absence.
//	adbPresent  -> say nothing. The phone is connected and configured; if adb
//	               cannot see it, adb is the problem, and the adb health
//	               machine already owns that message.
//	mtpAndroid  -> the phone is right there with debugging off.
//	otherwise   -> nothing Android-shaped on the bus.
func usbStateFrom(s usbSignals) UsbState {
	switch {
	case s.failed:
		return UsbUnknown
	case s.adbPresent:
		return UsbUnknown
	case s.mtpAndroid:
		return UsbDebuggingOff
	default:
		return UsbNoDevice
	}
}

// adbMarker is the compatible-id fragment that identifies the Android Debug
// Bridge interface.
//
// Verified on two unrelated devices: a Samsung SM-A556E (VID_04e8) and a Redmi
// Pad 2 (VID_2717 / VID_18d1). Windows does not register the interface under a
// dedicated SetupAPI class, so the compatible id is the only reliable marker.
//
// Note the inconsistent casing in the field itself: Samsung writes
// "Class_FF&SubClass_42", Xiaomi writes "Class_ff&SubClass_42". Every
// comparison against this constant is therefore case-insensitive, which
// TestMarkerMatchingIsCaseInsensitive exists to keep true.
const adbMarker = "class_ff&subclass_42&prot_01"

// mtpMarker is the compatible-id fragment of an MTP interface — Android's
// file-transfer mode, which a phone exposes whether or not USB debugging is on.
//
// This is the "the phone is really there" signal. It is not Android-specific,
// which is exactly why it cannot be used on its own: cameras, portable players
// and e-readers are MTP too. See androidVendors.
const mtpMarker = "class_06&subclass_01&prot_01"

// androidVendors is the set of USB vendor IDs Reflexo will accept as "this is
// an Android phone".
//
// It is a whitelist on purpose, and the direction of its failure is the whole
// design decision: a vendor missing from this list makes Reflexo fall back to
// "no phone" — the behaviour that existed before ADR-0007 — while a vendor
// wrongly *included* would make Reflexo call somebody's digital camera a
// phone. Under-claiming is a cosmetic miss; over-claiming tells the user to
// go and enable debugging on a device that has no such setting.
//
// Every entry is a real OEM vendor ID that has shipped Android hardware with
// USB debugging. When an unrecognised Android phone is seen, the log records
// its VID/PID so the list can be extended from evidence rather than from a
// guess.
var androidVendors = map[uint16]string{
	0x04e8: "Samsung",
	0x0bb4: "HTC",
	0x0b05: "Asus",
	0x12d1: "Huawei",
	0x18d1: "Google",
	0x1d6b: "Realme",
	0x22b8: "Motorola",
	0x22d9: "OPPO",
	0x2717: "Xiaomi",
	0x2a70: "OnePlus",
	0x2d95: "vivo",
	0x054c: "Sony",
	0x1004: "LG",
}

// vendorFromHardwareID extracts the USB vendor ID from a SetupAPI hardware id.
//
// Windows reports these in the form "USB\VID_04E8&PID_6860&MI_00", and the same
// value repeats for each interface of the same physical device, so every
// function carries its own vendor and no parent lookup is needed.
func vendorFromHardwareID(hardwareID string) (uint16, bool) {
	i := strings.Index(strings.ToLower(hardwareID), "vid_")
	if i < 0 {
		return 0, false
	}
	rest := hardwareID[i+len("vid_"):]
	if len(rest) < 4 {
		return 0, false
	}
	var vid uint16
	for _, c := range []byte(rest[:4]) {
		var nibble byte
		switch {
		case c >= '0' && c <= '9':
			nibble = c - '0'
		case c >= 'a' && c <= 'f':
			nibble = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			nibble = c - 'A' + 10
		default:
			return 0, false
		}
		vid = vid<<4 | uint16(nibble)
	}
	return vid, true
}

// hasMarker reports whether any of the given compatible ids contains marker.
//
// The whole buffer is joined first and then matched as one string. SetupAPI
// returns compatible ids as a REG_MULTI_SZ, and a per-element comparison would
// silently miss a device whose id list Windows decided to fold into a single
// value with semicolons — which is what this machine does for hardware ids.
func hasMarker(compatibleIDs []string, marker string) bool {
	return strings.Contains(strings.ToLower(strings.Join(compatibleIDs, ";")), marker)
}

// isAndroidPhone reports whether a USB interface belongs to a phone Reflexo
// recognises as Android, based on its vendor id.
func isAndroidPhone(hardwareID string) bool {
	vid, ok := vendorFromHardwareID(hardwareID)
	if !ok {
		return false
	}
	_, known := androidVendors[vid]
	return known
}
