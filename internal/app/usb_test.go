package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reflexo/internal/device"
	"reflexo/internal/guide"
	"reflexo/internal/i18n"
)

// withUSBProbe installs a probe that answers from a table instead of from the
// machine's physical USB bus.
//
// Inheriting newHealthApp matters twice over: it moves the App past the startup
// grace window, and it proves the probe is consulted on the ordinary healthy-
// adb path rather than only while recovering.
func withUSBProbe(t *testing.T, a *App, state device.UsbState, calls *int) *App {
	t.Helper()
	a.usbProbe = func() device.UsbState {
		if calls != nil {
			*calls++
		}
		return state
	}
	return a
}

// TestAPhoneOnTheBusChangesWhatTheScreenSays is the ADR-0007 payoff test: the
// probe result must reach the rendered instruction, not just a log line.
//
// Expectations come from the catalog keys rather than from literal Portuguese,
// because the instruction is translated in one place and this test is about the
// wiring, not about the wording.
func TestAPhoneOnTheBusChangesWhatTheScreenSays(t *testing.T) {
	tests := []struct {
		name  string
		probe device.UsbState
		want  string
	}{
		{"nada no barramento", device.UsbNoDevice, "guide.no_device.headline"},
		{"celular com depuracao desligada", device.UsbDebuggingOff, "guide.debugging_off.headline"},
		{"probe sem informacao", device.UsbUnknown, "guide.no_device.headline"},
	}

	for _, lang := range i18n.Available() {
		for _, tt := range tests {
			t.Run(string(lang)+"/"+tt.name, func(t *testing.T) {
				a := newHealthApp(t)
				withUSBProbe(t, a, tt.probe, nil)

				s := a.checkADB(context.Background(), nil, nil)
				if len(s.Devices) != 0 {
					t.Fatalf("o teste mandou uma lista vazia de aparelhos, veio %d", len(s.Devices))
				}

				a.apply(instructionFor(t, s, lang))

				if got, want := a.Snapshot().Headline, i18n.T(lang, tt.want); got != want {
					t.Errorf("Headline = %q; esperava %q", got, want)
				}
			})
		}
	}
}

// TestTheProbeIsNeverAskedWhenItCouldNotBeTrusted pins the call sites.
//
// The probe answers a question about the physical world, so calling it when the
// world is known to be in a different state invites a wrong answer to be
// believed. Each row is a state where the USB bus is simply not the subject.
func TestTheProbeIsNeverAskedWhenItCouldNotBeTrusted(t *testing.T) {
	tests := []struct {
		name      string
		devices   []device.Device
		listErr   error
		wantCalls int
	}{
		{
			name:      "adb respondendo sem aparelhos: e exatamente a pergunta",
			wantCalls: 1,
		},
		{
			name:      "celular visivel: nao ha nada a explicar",
			devices:   []device.Device{{Serial: "TEST123", State: device.StateReady}},
			wantCalls: 0,
		},
		{
			name:      "adb falhando: o aparelho pode estar inocente",
			listErr:   errDeadDaemon,
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newHealthApp(t)
			calls := 0
			withUSBProbe(t, a, device.UsbDebuggingOff, &calls)

			a.checkADB(context.Background(), tt.devices, tt.listErr)

			if calls != tt.wantCalls {
				t.Errorf("o probe do USB foi chamado %d vez(es); esperava %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestTheProbeIsLoggedOnChangeRatherThanEveryPoll keeps the support log
// readable. Polling runs every two seconds; a line per poll would bury
// everything else in the file.
func TestTheProbeIsLoggedOnChangeRatherThanEveryPoll(t *testing.T) {
	a, dir := newHealthAppWithLog(t)
	withUSBProbe(t, a, device.UsbNoDevice, nil)

	for i := 0; i < 5; i++ {
		a.debuggingOff()
	}

	if got := countLogLines(t, dir, "usb: nenhum"); got != 1 {
		t.Errorf("cinco polls iguais produziram %d linhas de log; esperava 1", got)
	}

	// And a real change must still be recorded.
	withUSBProbe(t, a, device.UsbDebuggingOff, nil)
	a.debuggingOff()

	if got := countLogLines(t, dir, "usb: "); got != 2 {
		t.Errorf("apos a mudanca de estado o log tem %d linhas de usb; esperava 2", got)
	}
}

// instructionFor renders the situation exactly the way Refresh does, so these
// tests exercise the real guide package instead of a local imitation.
func instructionFor(t *testing.T, s guide.Situation, lang i18n.Lang) guide.Instruction {
	t.Helper()
	return guide.Of(s, lang)
}

// countLogLines counts non-empty lines containing needle.
func countLogLines(t *testing.T, dir, needle string) int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "reflexo.log"))
	if err != nil {
		t.Fatalf("nao foi possivel ler o log: %v", err)
	}
	n := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, needle) {
			n++
		}
	}
	return n
}
