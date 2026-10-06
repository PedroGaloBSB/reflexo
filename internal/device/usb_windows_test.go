//go:build windows

package device

import "testing"

// TestScanUsbObservesTheRealBus proves the raw walk works, without asserting
// anything about what happens to be plugged in.
//
// This test exists because of an ambiguity that cost real time. UsbAndroidState
// returns UsbUnknown both when the walk works and finds an ADB interface, and
// when the walk fails outright. A green "does not panic" test therefore cannot
// tell a working detector from a broken one. Calling scanUsb directly and
// logging what it saw closes that gap: on a machine with an authorised phone,
// scanned is large and adbPresent is true, which is only reachable if the Setup
// API plumbing is genuinely correct.
//
// The assertions are on invariants that hold for any bus, including an empty
// one, and never on the contents. A build machine with nothing plugged in must
// still pass.
func TestScanUsbObservesTheRealBus(t *testing.T) {
	s := scanUsb()

	if s.failed {
		t.Fatal("a enumeracao do barramento USB falhou; " +
			"o caminho feliz de UsbAndroidState nunca sera alcancado nesta maquina")
	}
	if s.scanned == 0 && s.adbPresent {
		t.Error("adbPresent com scanned = 0: os sinais nao podem vir de um dispositivo inexistente")
	}
	if s.adbPresent && s.mtpForeign == nil {
		t.Log("nenhum dispositivo MTP desconhecido conectado (o esperado em geral)")
	}

	t.Logf("dispositivos presentes lidos: %d | adb: %t | mtp android: %t | mtp desconhecidos: %v",
		s.scanned, s.adbPresent, s.mtpAndroid, s.mtpForeign)
	t.Logf("conclusao: %s", usbStateFrom(s))
}

// TestForeignFileTransferDevicesAreRecorded is the other half of the whitelist
// contract: when a file-transfer device is not recognised, its vendor has to end
// up in the log, otherwise androidVendors can only ever grow by guesswork.
func TestForeignFileTransferDevicesAreRecorded(t *testing.T) {
	s := scanUsb()

	for _, vid := range s.mtpForeign {
		if len(vid) != 4 {
			t.Errorf("vendor desconhecido registrado como %q; esperado o formato vid sem '0x'", vid)
		}
		if _, known := androidVendors[parseHex16(vid)]; known {
			t.Errorf("vendor %s foi registrado como desconhecido mas esta em androidVendors", vid)
		}
	}
}

func parseHex16(s string) uint16 {
	var v uint16
	for _, c := range []byte(s) {
		var n byte
		switch {
		case c >= '0' && c <= '9':
			n = c - '0'
		case c >= 'a' && c <= 'f':
			n = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			n = c - 'A' + 10
		default:
			return 0
		}
		v = v<<4 | uint16(n)
	}
	return v
}
