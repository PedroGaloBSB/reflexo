//go:build windows

package device

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// This file reads Windows' own answer to "is a phone plugged in?", without
// running a subprocess and without asking for anything the user does not
// already have.
//
// Three attempts were made before this one, and all three are worth recording
// because each looks correct and is not:
//
//   - Enumerating the ADB interface by SetupAPI class returns nothing useful.
//     Windows registers it under the generic USBDevice class
//     {88bae032-5a81-49f0-bc3d-a4ff138216d6}; "ADB Interface" is only a
//     friendly name the driver installs, not a class of its own.
//   - Walking to the parent device to ask whether a composite is "complete"
//     fails here for a reason that has nothing to do with USB. The parent
//     instance reports Present: False while its own children report
//     Present: True, on a healthy machine with the phone plugged in. Whatever
//     that inconsistency is, it cannot be trusted to drive a message.
//
// What does work is the pair of markers in usb.go, both read off the
// interface itself: MTP for "a phone is really there", and the ADB compatible
// id for "the phone is answering".

var (
	setupapi = syscall.NewLazyDLL("setupapi.dll")

	procGetClassDevs      = setupapi.NewProc("SetupDiGetClassDevsW")
	procEnumDeviceInfo    = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procGetDevRegProperty = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procDestroyInfoList   = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	// digcfPresent restricts the set to devices that are actually connected.
	//
	// This flag is not an optimisation, it is the difference between working
	// and not. The USB enum tree keeps every device this machine has ever seen,
	// so a Redmi Note 9S that was plugged in months ago still has its whole
	// interface list sitting there. Without DIGCF_PRESENT that ghost would be
	// enough to make Reflexo claim a phone that is not there.
	digcfPresent = 0x00000002

	// digcfAllClasses is required because we pass no class filter; with a nil
	// class GUID Windows only enumerates the "default" classes otherwise.
	digcfAllClasses = 0x00000004

	spdrpHardwareID = 0x00000001
	spdrpCompatID   = 0x00000002

	invalidHandleValue = ^uintptr(0)
)

// spDevInfoData mirrors the Windows SP_DEVINFO_DATA.
//
// The layout is reproduced rather than imported because Go's alignment rules
// match C's on both amd64 (32 bytes) and 386 (28 bytes), so a plain struct is
// byte-correct without padding tricks.
type spDevInfoData struct {
	cbSize    uint32
	classGUID [16]byte
	devInst   uint32
	reserved  uintptr
}

// UsbAndroidState reports what can be concluded about a phone on the USB bus.
func UsbAndroidState() UsbState {
	return usbStateFrom(scanUsb())
}

// scanUsb walks Windows' own device list and records what it sees.
//
// It reads the PnP tree directly rather than running a subprocess, and asks for
// nothing the user does not already have. The cost is one enumeration of every
// present device and up to two registry property reads for each; on this
// machine, with 122 devices present, the walk measured well under a
// millisecond — cheap, though the caller still caches it because polling runs
// every two seconds and the answer does not change that fast.
func scanUsb() usbSignals {
	set, _, _ := procGetClassDevs.Call(0, 0, 0, digcfPresent|digcfAllClasses)
	if set == 0 || set == invalidHandleValue {
		// No enumeration means no knowledge. usbStateFrom turns this into
		// UsbUnknown, which is the only honest answer available.
		return usbSignals{failed: true}
	}
	defer procDestroyInfoList.Call(set)

	var signals usbSignals
	data := spDevInfoData{cbSize: uint32(unsafe.Sizeof(spDevInfoData{}))}

	for i := uint32(0); ; i++ {
		ok, _, _ := procEnumDeviceInfo.Call(set, uintptr(i), uintptr(unsafe.Pointer(&data)))
		if ok == 0 {
			// ERROR_NO_MORE_ITEMS ends the walk; any other failure ends it too,
			// and whatever was gathered up to that point is still the best
			// answer available.
			break
		}

		compat, found := readProperty(set, &data, spdrpCompatID)
		if !found {
			continue
		}
		signals.scanned++
		compatIDs := decodeMultiString(compat)

		if hasMarker(compatIDs, adbMarker) {
			signals.adbPresent = true
			// The walk deliberately continues. Returning here would be a
			// micro-optimisation on a bus that takes well under a millisecond,
			// and it would cost something that matters far more: the ability to
			// observe both signals at once, which is the only way to prove that
			// the file-transfer half of the detector works on a real phone. The
			// conclusion prioritises adbPresent anyway, in usbStateFrom.
			continue
		}

		if !hasMarker(compatIDs, mtpMarker) {
			continue
		}

		hardware, found := readProperty(set, &data, spdrpHardwareID)
		if !found {
			continue
		}
		ids := decodeMultiString(hardware)
		id := firstString(ids)

		if isAndroidPhone(id) {
			signals.mtpAndroid = true
			continue
		}

		// A file-transfer device that is not a phone we know — a camera, most
		// likely. Recorded rather than counted so the log can name it: this is
		// the evidence that grows androidVendors.
		if vid, ok := vendorFromHardwareID(id); ok {
			signals.mtpForeign = append(signals.mtpForeign, fmt.Sprintf("%04x", vid))
		}
	}

	return signals
}

// readProperty reads one SetupAPI registry property for a device.
//
// The call is made twice on purpose. Windows reports the required size instead
// of filling the buffer when given a zero-length one, and there is no way to
// know that size beforehand — device trees hold interface descriptions of
// wildly different lengths.
func readProperty(set uintptr, data *spDevInfoData, property uint32) ([]uint16, bool) {
	var regType uint32
	var size uint32

	// Sizing pass. A failure here that still reports no required size means
	// the property does not exist for this device, which is normal — not every
	// interface carries a hardware id.
	_, _, _ = procGetDevRegProperty.Call(
		set,
		uintptr(unsafe.Pointer(data)),
		uintptr(property),
		uintptr(unsafe.Pointer(&regType)),
		0, 0,
		uintptr(unsafe.Pointer(&size)),
	)
	if size == 0 {
		return nil, false
	}

	// Two extra elements of slack, so a value ending exactly on the buffer
	// size cannot run off the end when the terminating NUL is appended.
	buffer := make([]uint16, size/2+2)

	ok, _, _ := procGetDevRegProperty.Call(
		set,
		uintptr(unsafe.Pointer(data)),
		uintptr(property),
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 {
		return nil, false
	}
	return buffer, true
}

// decodeMultiString turns a REG_MULTI_SZ into its parts.
//
// Empty strings are dropped rather than kept: Windows pads the buffer with
// NULs, and an empty entry carries no information worth propagating.
func decodeMultiString(buffer []uint16) []string {
	var (
		parts []string
		cur   []uint16
	)
	for _, c := range buffer {
		if c == 0 {
			if len(cur) > 0 {
				parts = append(parts, string(utf16.Decode(cur)))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, c)
	}
	if len(cur) > 0 {
		parts = append(parts, string(utf16.Decode(cur)))
	}
	return parts
}

// firstString returns the first value, or "" when there is none.
//
// Hardware ids come back as several entries separated by "; ", all repeating the
// same VID, so taking the first is enough and avoids depending on the order.
func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
