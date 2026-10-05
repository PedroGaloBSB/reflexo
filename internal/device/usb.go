package device

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
// and as a serial port, and it simply never advertises the ADB interface. That
// was observed on a real SM-A556E where PnP showed MI_00 (WPD) and MI_01
// (serial) present while MI_03 (ADB Interface) was absent.
//
// Telling the user "no phone found" there is worse than useless — the phone is
// physically in front of them. It is the one situation where the product makes
// the user doubt reality, and it is the failure case a beginner hits most.
//
// The cost of finding out is low, because Windows and Linux both keep the
// answer in a place that can be read without a subprocess.
type UsbState int

const (
	// UsbUnknown means the platform gives us no way to tell, so nothing should
	// be claimed. This is the honest default: a missing implementation must
	// never turn into a wrong message.
	UsbUnknown UsbState = iota

	// UsbNoDevice means no Android device is present on the USB bus.
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
