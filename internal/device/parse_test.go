package device

import (
	"testing"
)

// Samples below are verbatim shapes emitted by adb 1.0.41, the version bundled
// with the pinned scrcpy release. They include the daemon chatter that adb
// mixes into the same stream on a cold machine, because a new user must never
// see a parse error for it.
func TestParseDevices(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Device
	}{
		{
			// Exactly what a machine with nothing plugged in prints.
			name: "nenhum aparelho",
			in:   "List of devices attached\n\n",
			want: nil,
		},
		{
			name: "pronto para espelhar",
			in: "List of devices attached\n" +
				"R5CT30XXXXX   device product:beyond1lte model:SM_G973F device:beyond1 transport_id:3\n",
			want: []Device{{Serial: "R5CT30XXXXX", State: StateReady}},
		},
		{
			name: "aguardando autorizacao",
			in: "List of devices attached\n" +
				"R5CT30XXXXX   unauthorized\n",
			want: []Device{{Serial: "R5CT30XXXXX", State: StateUnauthorized}},
		},
		{
			name: "offline",
			in: "List of devices attached\n" +
				"emulator-5554  offline\n",
			want: []Device{{Serial: "emulator-5554", State: StateOffline}},
		},
		{
			// "adb devices" without -l separates the columns with a tab, while
			// "-l" pads them with spaces. Splitting on the space alone dropped
			// every tab-separated line in silence.
			name: "separado por tab",
			in: "List of devices attached\n" +
				"R5CT30XXXXX\tdevice\n",
			want: []Device{{Serial: "R5CT30XXXXX", State: StateReady}},
		},
		{
			name: "tab com campos extras",
			in: "List of devices attached\n" +
				"R5CT30XXXXX\tunauthorized\n",
			want: []Device{{Serial: "R5CT30XXXXX", State: StateUnauthorized}},
		},
		{
			// A line with no state column is not a device. Inventing one would
			// produce a phone Reflexo cannot describe.
			name: "sem coluna de estado",
			in: "List of devices attached\n" +
				"R5CT30XXXXX\n",
			want: nil,
		},
		{
			name: "dois aparelhos",
			in: "List of devices attached\n" +
				"AAA111   device product:x model:Y transport_id:1\n" +
				"BBB222   device product:z model:W transport_id:2\n",
			want: []Device{
				{Serial: "AAA111", State: StateReady},
				{Serial: "BBB222", State: StateReady},
			},
		},
		{
			// adb starts its daemon on first use and prints this before the list.
			name: "daemon frio nao vira aparelho",
			in: "* daemon not running; starting now at tcp:5037\n" +
				"* daemon started successfully\n" +
				"List of devices attached\n" +
				"CCC333   device\n",
			want: []Device{{Serial: "CCC333", State: StateReady}},
		},
		{
			// Seen when the USB driver is missing or the user lacks the plugdev
			// group on Linux.
			name: "sem permissao e ignorado",
			in: "List of devices attached\n" +
				"????????????\tno permissions (user in plugdev group; are your udev rules wrong?)\n",
			want: nil,
		},
		{
			name: "saida vazia de adb que falhou",
			in:   "",
			want: nil,
		},
		{
			name: "estado desconhecido e preservado",
			in: "List of devices attached\n" +
				"DDD444   somethingnew\n",
			want: []Device{{Serial: "DDD444", State: StateUnknown}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDevices(tt.in)

			if len(got) != len(tt.want) {
				t.Fatalf("ParseDevices() retornou %d aparelhos, esperado %d: %+v",
					len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i].Serial != tt.want[i].Serial {
					t.Errorf("aparelho %d: serial = %q, esperado %q",
						i, got[i].Serial, tt.want[i].Serial)
				}
				if got[i].State != tt.want[i].State {
					t.Errorf("aparelho %d: estado = %q, esperado %q",
						i, got[i].State, tt.want[i].State)
				}
			}
		})
	}
}

func TestParseProps(t *testing.T) {
	// Shape taken from a real Samsung getprop dump.
	in := `[ro.product.manufacturer]: [samsung]
[ro.product.model]: [SM-A556B]
[ro.product.brand]: [samsung]
[ro.build.version.release]: [15]
[ro.build.version.sdk]: [35]
[persist.sys.locale]: []
[weird.key]: [value with spaces]
`

	got := ParseProps(in)

	want := map[string]string{
		"ro.product.manufacturer":  "samsung",
		"ro.product.model":         "SM-A556B",
		"ro.build.version.release": "15",
		"ro.build.version.sdk":     "35",
		"persist.sys.locale":       "",
		"weird.key":                "value with spaces",
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("ParseProps()[%q] = %q, esperado %q", k, got[k], v)
		}
	}
}

func TestParsePropsIgnoresGarbage(t *testing.T) {
	// getprop can emit warnings before the payload.
	in := "WARNING: linker: unused DT entry\n" +
		"[ro.product.model]: [Redmi Note 9S]\n" +
		"lshal: not found\n" +
		"malformed line without brackets\n"

	got := ParseProps(in)

	if got["ro.product.model"] != "Redmi Note 9S" {
		t.Errorf("propriedade válida foi perdida: %+v", got)
	}
	if len(got) != 1 {
		t.Errorf("esperava apenas 1 propriedade válida, obtive %d: %+v", len(got), got)
	}
}

// Device identity helpers read from Props, which is how the guide builds the
// user-facing label.
func TestDeviceIdentity(t *testing.T) {
	t.Run("xiaomi via marca secundaria", func(t *testing.T) {
		d := Device{Props: map[string]string{
			"ro.product.manufacturer": "Xiaomi",
			"ro.product.brand":        "Redmi",
			"ro.product.model":        "Redmi Note 9S",
		}}
		if got := d.Brand(); got != "xiaomi" {
			t.Errorf("Brand() = %q, esperado %q", got, "xiaomi")
		}
		if got := d.DisplayName(); got != "Redmi Note 9S" {
			t.Errorf("DisplayName() = %q, esperado %q", got, "Redmi Note 9S")
		}
	})

	t.Run("samsung por fabricante", func(t *testing.T) {
		d := Device{Props: map[string]string{
			"ro.product.manufacturer": "samsung",
			"ro.product.model":        "SM-A556B",
		}}
		if got := d.Brand(); got != "samsung" {
			t.Errorf("Brand() = %q, esperado %q", got, "samsung")
		}
	})

	t.Run("versao android com fallback para codename", func(t *testing.T) {
		d := Device{Props: map[string]string{
			"ro.build.version.codename": "REL",
		}}
		if got := d.AndroidRelease(); got != "REL" {
			t.Errorf("AndroidRelease() = %q, esperado %q", got, "REL")
		}
	})

	t.Run("aparelho sem propriedades nao quebra", func(t *testing.T) {
		var d Device
		if got := d.DisplayName(); got != "" {
			t.Errorf("DisplayName() = %q, esperado vazio", got)
		}
		if got := d.Brand(); got != "" {
			t.Errorf("Brand() = %q, esperado vazio", got)
		}
	})
}

func TestStateReady(t *testing.T) {
	if !StateReady.Ready() {
		t.Error("StateReady.Ready() = false, esperado true")
	}
	for _, s := range []State{StateNone, StateUnauthorized, StateOffline, StateRecovery, StateBootloader, StateUnknown} {
		if s.Ready() {
			t.Errorf("%q.Ready() = true, esperado false", s)
		}
	}
}
