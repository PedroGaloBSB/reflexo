package guide

import (
	"strings"
	"testing"

	"reflexo/internal/device"
	"reflexo/internal/i18n"
)

// props builds a Device with the given properties, in StateReady unless told
// otherwise.
func props(state device.State, kv map[string]string) device.Device {
	d := device.Device{Serial: "TEST123", State: state, Props: map[string]string{}}
	for k, v := range kv {
		d.Props[k] = v
	}
	return d
}

// The table below is run for every shipped language. A guide test that only
// checked Portuguese would let an English catalog drift into the UI unnoticed.
func TestOf(t *testing.T) {
	samsung := props(device.StateReady, map[string]string{
		"ro.product.manufacturer":  "samsung",
		"ro.product.model":         "SM-A556B",
		"ro.build.version.release": "15",
		"ro.build.version.sdk":     "35",
	})
	redmi := props(device.StateReady, map[string]string{
		"ro.product.manufacturer":  "Xiaomi",
		"ro.product.brand":         "Redmi",
		"ro.product.model":         "Redmi Note 9S",
		"ro.build.version.release": "12",
		"ro.build.version.sdk":     "31",
	})

	tests := []struct {
		name string
		sit  Situation
		want Severity
	}{
		{"sem adb", Situation{ADBAvailable: false}, SeverityWarn},
		{"nenhum aparelho", Situation{ADBAvailable: true}, SeverityIdle},
		{
			"aguardando autorizacao",
			Situation{ADBAvailable: true, Devices: []device.Device{
				props(device.StateUnauthorized, nil),
			}},
			SeverityAction,
		},
		{
			"offline",
			Situation{ADBAvailable: true, Devices: []device.Device{
				props(device.StateOffline, nil),
			}},
			SeverityWarn,
		},
		{
			"samsung pronto",
			Situation{ADBAvailable: true, Devices: []device.Device{samsung}},
			SeverityReady,
		},
		{
			"xiaomi pronto",
			Situation{ADBAvailable: true, Devices: []device.Device{redmi}},
			SeverityReady,
		},
		{
			"dois aparelhos",
			Situation{ADBAvailable: true, Devices: []device.Device{samsung, redmi}},
			SeverityAction,
		},
		{
			"estado desconhecido",
			Situation{ADBAvailable: true, Devices: []device.Device{
				props(device.StateUnknown, nil),
			}},
			SeverityWarn,
		},
		{
			"modo recovery",
			Situation{ADBAvailable: true, Devices: []device.Device{
				props(device.StateRecovery, nil),
			}},
			SeverityWarn,
		},
	}

	for _, lang := range i18n.Available() {
		for _, tt := range tests {
			t.Run(string(lang)+"/"+tt.name, func(t *testing.T) {
				got := Of(tt.sit, lang)

				if got.Severity != tt.want {
					t.Errorf("Severity = %v, esperado %v", got.Severity, tt.want)
				}
				assertUsable(t, got)
			})
		}
	}
}

// assertUsable captures the invariants that hold in every language. Go's %!s
// markers and raw i18n keys would both be catastrophic in a guided product.
func assertUsable(t *testing.T, in Instruction) {
	t.Helper()

	fields := map[string]string{
		"Headline": in.Headline,
		"Detail":   in.Detail,
	}
	for name, v := range fields {
		switch {
		case strings.TrimSpace(v) == "":
			t.Errorf("%s está vazio", name)
		case strings.Contains(v, "%!"):
			t.Errorf("%s tem erro de formatação: %q", name, v)
		case strings.Contains(v, "guide.") || strings.Contains(v, "setup."):
			t.Errorf("%s expõe uma chave i18n crua: %q", name, v)
		}
	}

	if len(in.Steps) == 0 {
		t.Error("toda instrução deve ter pelo menos um passo")
	}
	for i, s := range in.Steps {
		if strings.TrimSpace(s) == "" {
			t.Errorf("passo %d está vazio", i)
		}
		if strings.Contains(s, "%!") {
			t.Errorf("passo %d tem erro de formatação: %q", i, s)
		}
	}
}

// Only a fully authorised device may start mirroring. Getting this wrong means
// either a dead-end button or a confusing crash later.
func TestCanStartOnlyWhenReady(t *testing.T) {
	states := []device.State{
		device.StateNone, device.StateUnauthorized, device.StateOffline,
		device.StateRecovery, device.StateBootloader, device.StateUnknown,
	}
	for _, lang := range i18n.Available() {
		for _, s := range states {
			in := Of(Situation{ADBAvailable: true, Devices: []device.Device{
				props(s, map[string]string{"ro.product.model": "Teste"}),
			}}, lang)
			if in.CanStart {
				t.Errorf("%s estado %q: CanStart = true, esperado false", lang, s)
			}
		}

		ready := Of(Situation{ADBAvailable: true, Devices: []device.Device{
			props(device.StateReady, map[string]string{"ro.product.model": "Teste"}),
		}}, lang)
		if !ready.CanStart {
			t.Errorf("%s estado ready: CanStart = false, esperado true", lang)
		}
	}
}

// The vendor warning must appear for Xiaomi-family devices and stay away for
// everyone else, or users learn to ignore it.
func TestVendorNoteScoping(t *testing.T) {
	tests := []struct {
		name         string
		manufacturer string
		brand        string
		wantNote     bool
	}{
		{"xiaomi", "Xiaomi", "Redmi", true},
		{"poco", "Xiaomi", "poco", true},
		{"redmi direto", "Xiaomi", "", true},
		{"samsung", "samsung", "samsung", false},
		{"google", "Google", "google", false},
		{"desconhecido", "", "", false},
	}

	for _, lang := range i18n.Available() {
		for _, tt := range tests {
			t.Run(string(lang)+"/"+tt.name, func(t *testing.T) {
				d := props(device.StateReady, map[string]string{
					"ro.product.manufacturer":  tt.manufacturer,
					"ro.product.brand":         tt.brand,
					"ro.product.model":         "Modelo X",
					"ro.build.version.release": "15",
				})

				in := Of(Situation{ADBAvailable: true, Devices: []device.Device{d}}, lang)

				// The check compares against the catalog entry itself rather
				// than a word in it, so rewording the advice in either
				// language cannot silently break the test.
				marker := i18n.T(lang, "guide.vendor.xiaomi.step1")

				if contains(in.Steps, marker) != tt.wantNote {
					t.Errorf("aviso presente = %v, esperado %v\nSteps: %v",
						contains(in.Steps, marker), tt.wantNote, in.Steps)
				}
			})
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Devices with no readable model must still produce a usable label rather than
// an empty headline.
func TestHeadlineNeverEmpty(t *testing.T) {
	inputs := []map[string]string{
		nil,
		{},
		{"ro.product.model": ""},
		{"ro.product.name": "sdk_gphone64_x86_64"},
		{"ro.product.model": "Pixel 8"},
		{"ro.product.model": "SM-A556B"},
		{"ro.product.model": "a"},
	}

	for _, lang := range i18n.Available() {
		for _, kv := range inputs {
			d := props(device.StateReady, kv)
			in := Of(Situation{ADBAvailable: true, Devices: []device.Device{d}}, lang)

			if strings.TrimSpace(in.Headline) == "" {
				t.Errorf("%s props %v gerou Headline vazia", lang, kv)
			}
		}
	}
}

// Model codes must not be mangled into looking like words.
func TestTitleizeLeavesModelCodesAlone(t *testing.T) {
	tests := []struct{ in, want string }{
		{"galaxy a55", "Galaxy a55"},
		{"SM-A556B", "SM-A556B"},
		{"PIXEL 8", "PIXEL 8"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := titleize(tt.in); got != tt.want {
			t.Errorf("titleize(%q) = %q, esperado %q", tt.in, got, tt.want)
		}
	}
}

// The count in the "several phones" headline must be interpolated, not printed
// as a verb.
func TestMultipleDevicesInterpolatesCount(t *testing.T) {
	for _, lang := range i18n.Available() {
		for _, n := range []int{2, 3, 10} {
			in := Of(Situation{ADBAvailable: true, Devices: make([]device.Device, n)}, lang)

			if strings.Contains(in.Headline, "%!") {
				t.Errorf("%s com %d aparelhos: formatação quebrada: %q", lang, n, in.Headline)
			}
			if !strings.Contains(in.Headline, itoa(n)) {
				t.Errorf("%s com %d aparelhos: contagem ausente em %q", lang, n, in.Headline)
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A device with no properties must not produce a Detail that claims to know the
// Android version.
func TestDetailWithoutProps(t *testing.T) {
	for _, lang := range i18n.Available() {
		d := props(device.StateReady, map[string]string{"ro.product.model": "Modelo"})
		in := Of(Situation{ADBAvailable: true, Devices: []device.Device{d}}, lang)

		if strings.Contains(in.Detail, "API") || strings.Contains(in.Detail, "%!") {
			t.Errorf("%s: Detail afirma detalhes que não temos: %q", lang, in.Detail)
		}
	}
}

// TestNoDeviceChecksPhoneBeforeCable encodes a decision made after the first
// real-device test.
//
// `adb devices` returns an empty list in two very different situations: no
// phone at all, and a phone that is plugged in with USB debugging off. Reflexo
// cannot tell them apart, and the second case is by far the more common one.
// The first version opened with "Connect your phone with the USB cable" — which
// told a user with the cable already connected to do something they had just
// done.
//
// So the instruction must lead with the phone-side check. This test fails if
// someone reorders the steps back to leading with the cable.
func TestNoDeviceChecksPhoneBeforeCable(t *testing.T) {
	// Language-independent anchors: the step must talk about debugging on the
	// phone, and must not open with an instruction to connect the cable.
	debugAnchor := map[i18n.Lang]string{
		i18n.PortugueseBrazil: "depuração",
		i18n.English:          "debugging",
	}
	cableAnchor := map[i18n.Lang]string{
		i18n.PortugueseBrazil: "conecte o celular",
		i18n.English:          "plug the phone",
	}

	for _, lang := range i18n.Available() {
		in := Of(Situation{ADBAvailable: true}, lang)

		first := strings.ToLower(in.Steps[0])
		if strings.Contains(first, strings.ToLower(cableAnchor[lang])) {
			t.Errorf("%s: o primeiro passo assume que o cabo está fora (%q); "+
				"a causa mais comum é o celular conectado sem Depuração USB",
				lang, in.Steps[0])
		}
		if !strings.Contains(first, strings.ToLower(debugAnchor[lang])) {
			t.Errorf("%s: o primeiro passo deveria checar a Depuração USB no celular, "+
				"mas diz %q", lang, in.Steps[0])
		}
	}
}
