// Package device wraps the small slice of the Android Debug Bridge that
// Reflexo actually needs: listing attached devices and reading their
// properties.
//
// Everything here is pure parsing plus process invocation, kept free of UI
// concerns so it can be unit-tested without an Android device (see parse_test.go).
package device

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// State is the connection state reported by "adb devices".
type State string

const (
	// StateNone means no device is attached at all.
	StateNone State = ""
	// StateUnauthorized means the device is attached but the user has not yet
	// accepted the "Allow USB debugging?" prompt. This is the single most
	// common beginner blocker and needs its own instruction.
	StateUnauthorized State = "unauthorized"
	// StateOffline means the device was seen but adb lost contact.
	StateOffline State = "offline"
	// StateReady means the device is fully available.
	StateReady State = "device"
	// StateRecovery and StateBootloader are pre-Android states; mirroring is
	// impossible but we can still tell the user what happened.
	StateRecovery   State = "recovery"
	StateBootloader State = "bootloader"
	// StateUnknown covers states we do not model explicitly.
	StateUnknown State = "unknown"
)

// Ready reports whether the device can be mirrored right now.
func (s State) Ready() bool { return s == StateReady }

// Device is one attached Android device.
type Device struct {
	Serial string
	State  State

	// Raw properties from "getprop", already trimmed of the "[k]: [v]" wrapper.
	Props map[string]string
}

// Manufacturer is the vendor name, e.g. "Xiaomi", "samsung".
func (d Device) Manufacturer() string { return d.Props["ro.product.manufacturer"] }

// Model is the marketing model name, e.g. "Redmi Note 9S".
func (d Device) Model() string { return d.Props["ro.product.model"] }

// Brand maps Xiaomi's many sub-brands onto a single vendor family so the guide
// can apply the right vendor-specific advice.
func (d Device) Brand() string {
	m := strings.ToLower(d.Manufacturer())
	b := strings.ToLower(d.Props["ro.product.brand"])
	for _, v := range []string{m, b} {
		switch {
		case strings.Contains(v, "xiaomi"), strings.Contains(v, "redmi"),
			strings.Contains(v, "poco"):
			return "xiaomi"
		case strings.Contains(v, "samsung"):
			return "samsung"
		}
	}
	return m
}

// AndroidRelease is the user-facing version string, e.g. "15".
func (d Device) AndroidRelease() string {
	if v := d.Props["ro.build.version.release"]; v != "" {
		return v
	}
	return d.Props["ro.build.version.codename"]
}

// SDKInt is the API level, e.g. "35".
func (d Device) SDKInt() string { return d.Props["ro.build.version.sdk"] }

// DisplayName is the human label used in the UI, e.g. "Samsung Galaxy A55".
func (d Device) DisplayName() string {
	parts := make([]string, 0, 2)
	if m := strings.TrimSpace(d.Model()); m != "" {
		parts = append(parts, m)
	} else if n := strings.TrimSpace(d.Props["ro.product.name"]); n != "" {
		parts = append(parts, n)
	}
	return strings.Join(parts, " ")
}

// Client runs adb commands.
type Client struct {
	// ADBPath is the absolute path to the bundled adb executable.
	ADBPath string
	// Timeout bounds every adb invocation. Kept short because Reflexo polls
	// the device list continuously while the user is plugging things in.
	Timeout time.Duration
}

// NewClient builds a Client around an adb executable.
func NewClient(adbPath string) *Client {
	return &Client{ADBPath: adbPath, Timeout: 5 * time.Second}
}

// run executes adb with the given arguments and returns stdout and stderr
// combined, which is how adb reports some errors.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// #nosec G204 -- args come from Reflexo itself, never from user input.
	cmd := exec.CommandContext(ctx, c.ADBPath, args...)

	if runtime.GOOS == "windows" {
		// Without this, adb.exe can pop a console window of its own.
		cmd.SysProcAttr = hideConsole()
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("adb %s: %w", strings.Join(args, " "), err)
	}
	return buf.String(), nil
}

// List returns every attached device.
//
// A machine with nothing plugged in yields a nil slice and no error: "no
// device" is a normal state to display, not a failure.
func (c *Client) List(ctx context.Context) ([]Device, error) {
	out, err := c.run(ctx, "devices", "-l")
	if err != nil {
		// "adb devices" exits non-zero only on hard failures. Preserve the
		// text anyway: it usually explains the real problem.
		return nil, fmt.Errorf("%w: %s", err, firstLine(out))
	}
	return ParseDevices(out), nil
}

// Props reads all system properties from one device.
func (c *Client) Props(ctx context.Context, serial string) (map[string]string, error) {
	out, err := c.run(ctx, "-s", serial, "shell", "getprop")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, firstLine(out))
	}
	return ParseProps(out), nil
}

// StartServer launches the adb daemon. Reflexo calls this once at startup so a
// cold machine does not show a spurious error before adb is warm.
func (c *Client) StartServer(ctx context.Context) error {
	_, err := c.run(ctx, "start-server")
	return err
}

// firstLine trims adb output down to a single readable line for error text.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "(sem saída)"
}

// ParseDevices parses the output of "adb devices -l".
//
// It is deliberately tolerant: adb mixes diagnostics into the same stream, and
// a brand-new user should never see a parse failure when the real message is
// "no device attached".
func ParseDevices(out string) []Device {
	var devices []Device

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	seenHeader := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Skip the "List of devices attached" header and adb daemon chatter.
		if strings.HasPrefix(line, "List of devices") {
			seenHeader = true
			continue
		}
		if strings.HasPrefix(line, "*") || strings.HasPrefix(line, "adb:") {
			continue
		}
		// A permission problem is reported here, not in the state column.
		if strings.Contains(line, "no permissions") {
			continue
		}
		if !seenHeader {
			// Unexpected preamble; ignore rather than invent a device.
			continue
		}

		// Split on any whitespace rather than on a literal space. "adb devices"
		// separates the columns with a tab and "adb devices -l" pads them with
		// spaces, and cutting on " " alone silently dropped every tab-separated
		// line — which is the shape a future adb would most likely produce, and
		// the one this package documents itself as tolerating.
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "" {
			continue
		}

		// The state token is the second column; anything after it is
		// "key:value" hints we do not need because Props covers them.
		devices = append(devices, Device{
			Serial: fields[0],
			State:  normalizeState(fields[1]),
			Props:  map[string]string{},
		})
	}

	return devices
}

// normalizeState maps an adb state token onto our State type.
func normalizeState(s string) State {
	switch State(s) {
	case StateUnauthorized, StateOffline, StateReady, StateRecovery, StateBootloader:
		return State(s)
	case "":
		return StateNone
	default:
		return StateUnknown
	}
}

// ParseProps parses the output of "adb shell getprop", whose lines look like:
//
//	[ro.product.manufacturer]: [Xiaomi]
func ParseProps(out string) map[string]string {
	props := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		key, value, ok := parsePropLine(scanner.Text())
		if !ok {
			continue
		}
		props[key] = value
	}
	return props
}

// parsePropLine splits "[key]: [value]" into its two halves.
func parsePropLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return "", "", false
	}

	// Everything between the first "[" and the first "]".
	end := strings.Index(line, "]")
	if end < 0 {
		return "", "", false
	}
	key = line[1:end]

	rest := strings.TrimSpace(line[end+1:])
	if !strings.HasPrefix(rest, ":") {
		return "", "", false
	}

	value = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")

	return key, strings.TrimSpace(value), key != ""
}
