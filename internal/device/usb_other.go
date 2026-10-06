//go:build !windows

package device

// UsbAndroidState returns UsbUnknown on every platform except Windows, and that
// is a decision rather than a leftover.//
// The three states exist to stop Reflexo from telling someone that no phone is
// connected while their phone is sitting on the desk. That only works if the
// middle state is reachable and correct. A plausible-looking implementation
// that guesses would reintroduce the exact failure in a new place: the obvious
// Linux version reads a vendor id, decides "Android", and fires the moment
// anyone plugs in a device that happens to share that vendor id. A wrong
// message costs the user more than the silence it replaced.
//
// The policy that decides what counts as a phone already lives in usb.go and is
// pure, portable and unit-tested everywhere. What is missing on these platforms
// is only the plumbing — reading the bus without a subprocess. Both recipes
// are written down below so the work is a matter of writing an interpreter, not
// of rediscovering the rules.
//
// # Linux
//
// Everything needed is under /sys/bus/usb/devices, readable without privileges:
// the function directories are named "<bus>-<port>:<config>.<interface>" and
// each one exposes bInterfaceClass, bInterfaceSubClass and bInterfaceProtocol.
// A MTP function is 06/01/01 and an ADB function is ff/42/01, which are the
// same numbers as the compatible ids read on Windows. The device-level idVendor
// file next to them is the vendor for androidVendors.
//
// One trap worth writing down: /sys keeps entries for disconnected devices in
// some configurations, so the same "is it actually present" care that
// DIGCF_PRESENT provides on Windows is needed here, and its equivalent is not
// obvious. That is the reason this is not filled in blind.
//
// macOS
//
// The equivalent listing is ioreg -p IOUSB -w0 -l, whose "idVendor" and
// "USB Product Name" values would map onto the same policy. It requires
// spawning a process, which is the one thing this codebase otherwise refuses to
// do for a read-only check, and it cannot be validated from a machine where the
// only phone available is Android-over-USB on Windows.
// scanUsb reports that there is no way to look, which usbStateFrom turns into
// the honest "nothing to say".
func scanUsb() usbSignals { return usbSignals{failed: true} }

// UsbAndroidState is the platform-independent entry point.
func UsbAndroidState() UsbState { return usbStateFrom(scanUsb()) }
