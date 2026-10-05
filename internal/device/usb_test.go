package device

import "testing"

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
