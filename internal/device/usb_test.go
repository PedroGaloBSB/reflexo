package device

import (
	"strings"
	"testing"
)

// TestUsbStateFromConcludesOnlyWhatItSaw is the table that makes ADR-0007
// enforceable rather than aspirational.
//
// Each row is a way the product could lie to somebody holding a phone. The
// expected column is not "what the code finds convenient" but "what would still
// be defensible to read".
func TestUsbStateFromConcludesOnlyWhatItSaw(t *testing.T) {
	tests := []struct {
		name string
		in   usbSignals
		want UsbState
	}{
		{
			name: "no enumeration at all claims nothing",
			in:   usbSignals{failed: true},
			want: UsbUnknown,
		},
		{
			name: "an empty but readable bus means no phone",
			in:   usbSignals{scanned: 120},
			want: UsbNoDevice,
		},
		{
			// The most important row in this file. A phone that is plugged in
			// and correctly configured must never be told it has debugging
			// switched off, even though it also exposes the file-transfer
			// interface that the other branch keys on.
			name: "adb present overrides the file transfer interface",
			in:   usbSignals{scanned: 122, adbPresent: true, mtpAndroid: true},
			want: UsbUnknown,
		},
		{
			name: "a recognised phone exposing file transfer has debugging off",
			in:   usbSignals{scanned: 122, mtpAndroid: true},
			want: UsbDebuggingOff,
		},
		{
			// The failure mode a vendor whitelist is designed to absorb: an
			// unrecognised device is not a phone, it is simply not known to be
			// one.
			name: "a file transfer device from an unknown vendor is not a phone",
			in:   usbSignals{scanned: 122, mtpForeign: []string{"04e9"}},
			want: UsbNoDevice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usbStateFrom(tt.in); got != tt.want {
				t.Errorf("usbStateFrom(%+v) = %s; esperava %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestVendorFromHardwareIDReadsTheRealFormat pins the parser against strings
// taken from this machine's registry, not from documentation.
//
// Both spellings Windows uses are present: the interface path with a
// "&MI_00" suffix, and the same value repeated as the parent's. Getting the
// vendor wrong here is how an MTP camera ends up being announced as a phone.
func TestVendorFromHardwareIDReadsTheRealFormat(t *testing.T) {
	tests := []struct {
		in     string
		want   uint16
		wantOK bool
	}{
		{in: `USB\VID_04E8&PID_6860&MI_00`, want: 0x04e8, wantOK: true},
		{in: `USB\VID_04E8&PID_6860&MI_00 ; USB\VID_04E8&PID_6860&MI_00`, want: 0x04e8, wantOK: true},
		{in: `USB\VID_2717&PID_FF48&MI_01`, want: 0x2717, wantOK: true},
		{in: `USB\VID_18D1&PID_4EE7`, want: 0x18d1, wantOK: true},
		{in: `USB\VID_0000&PID_0000`, want: 0, wantOK: true},
		{in: `USB\Class_ff&SubClass_42&Prot_01`, wantOK: false},
		{in: `USB\VID_04&PID_1`, wantOK: false},
		{in: `USB\VID_zzzz&PID_6860`, wantOK: false},
		{in: "", wantOK: false},
	}

	for _, tt := range tests {
		got, ok := vendorFromHardwareID(tt.in)
		if ok != tt.wantOK {
			t.Errorf("vendorFromHardwareID(%q) ok = %v; esperava %v", tt.in, ok, tt.wantOK)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("vendorFromHardwareID(%q) = %#04x; esperava %#04x", tt.in, got, tt.want)
		}
	}
}

// TestMarkerMatchingIsCaseInsensitive exists because Windows itself is not
// consistent. A Samsung registered "Class_FF&SubClass_42&Prot_01" while a Redmi
// on the same bus registered "Class_ff&SubClass_42&Prot_01"; a case-sensitive
// comparison would find the first phone and miss the second, which is the worst
// possible shape of bug — it works on the developer's own device.
func TestMarkerMatchingIsCaseInsensitive(t *testing.T) {
	samsung := []string{
		"USB\\MS_COMP_WINUSB",
		"USB\\COMPAT_VID_04e8&Class_FF&SubClass_42&Prot_01",
		"USB\\Class_FF&SubClass_42&Prot_01",
	}
	xiaomi := []string{
		"USB\\MS_COMP_WINUSB",
		"USB\\COMPAT_VID_2717&Class_ff&SubClass_42&Prot_01",
		"USB\\Class_ff&SubClass_42&Prot_01",
	}
	keyboard := []string{
		"USB\\VID_1234&PID_5678&REV_0100",
		"USB\\Class_00&SubClass_00&Prot_00",
	}

	for name, ids := range map[string][]string{"samsung": samsung, "xiaomi": xiaomi} {
		if !hasMarker(ids, adbMarker) {
			t.Errorf("%s: o marcador de ADB nao foi encontrado em %v", name, ids)
		}
	}
	if hasMarker(keyboard, adbMarker) {
		t.Errorf("um teclado USB foi reconhecido como interface ADB: %v", keyboard)
	}
	if hasMarker(nil, adbMarker) {
		t.Error("uma lista vazia de compatibilidade casou com o marcador de ADB")
	}
}

// TestAForeignFileTransferDeviceIsNotAPhone guards the whitelist boundary.
//
// MTP is not an Android invention: cameras, portable players and e-readers all
// expose it. Treating any of them as a phone is the failure ADR-0007 was opened
// to prevent, because the resulting message tells the user to go and enable
// USB debugging on a device that has no such setting.
func TestAForeignFileTransferDeviceIsNotAPhone(t *testing.T) {
	mtp := []string{
		"USB\\MS_COMP_MTP",
		"USB\\COMPAT_VID_046d&Class_06&SubClass_01&Prot_01",
	}

	if !hasMarker(mtp, mtpMarker) {
		t.Fatal("um dispositivo MTP classico nao casou com o marcador; o teste nao esta exercitando nada")
	}
	if isAndroidPhone(`USB\VID_046D&PID_0779`) {
		t.Error("um dispositivo MTP de fabricante desconhecido foi aceito como celular Android")
	}

	// The counterpart: a real Android vendor must still be recognised, or the
	// whitelist silently breaks the feature it exists to protect.
	if !isAndroidPhone(`USB\VID_2717&PID_FF48&MI_00`) {
		t.Error("um aparelho Xiaomi real nao foi reconhecido como Android")
	}
}

// TestEveryKnownVendorHasAName keeps the log useful. androidVendors maps a
// vendor id to a human name specifically so a support bundle can be read
// without a lookup table, and an unnamed entry would defeat that.
func TestEveryKnownVendorHasAName(t *testing.T) {
	for vid, name := range androidVendors {
		if strings.TrimSpace(name) == "" {
			t.Errorf("vendor %#04x esta na lista sem nome", vid)
		}
	}
}

// TestUsbAndroidStateDoesNotPanic exercises the real Setup API on the machine
// running the test.
//
// It asserts almost nothing on purpose. The answer legitimately changes with
// whatever is plugged in, so a hard assertion would make the suite fail
// whenever someone charges their phone on the build machine. What must hold is
// that the call returns one of the three known states and does not crash — a
// Setup API mistake usually shows up as an access violation, not an error, and
// that is exactly what this guards against.
func TestUsbAndroidStateDoesNotPanic(t *testing.T) {
	got := UsbAndroidState()

	switch got {
	case UsbUnknown, UsbNoDevice, UsbDebuggingOff:
	default:
		t.Fatalf("UsbAndroidState() = %d, que nao e um estado conhecido", int(got))
	}

	t.Logf("USB detectado neste momento: %s", got)
}

// TestUsbStateStringNamesEveryState keeps the log readable. A support bundle is
// read by a person, so an integer in it is a wasted line.
func TestUsbStateStringNamesEveryState(t *testing.T) {
	for _, s := range []UsbState{UsbUnknown, UsbNoDevice, UsbDebuggingOff} {
		if s.String() == "" {
			t.Errorf("estado %d tem String() vazio", int(s))
		}
	}
	if UsbUnknown.String() != "desconhecido" {
		t.Errorf("UsbUnknown.String() = %q; um estado desconhecido precisa dizer isso "+
			"para nunca ser confundido com uma certeza", UsbUnknown.String())
	}
}
