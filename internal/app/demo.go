package app

import (
	"time"

	"reflexo/internal/device"
	"reflexo/internal/guide"
)

// The demonstration exists because of a specific failure of the product's
// first impression.
//
// Almost nobody who opens Reflexo has an Android phone plugged in. What they
// see is "no phone found", and a product that introduces itself by reporting an
// absence is not going to be interesting to anyone. So when there is nothing to
// show, the product shows what it would have shown.
//
// The rule that keeps this honest: the demonstration feeds the same
// guide.Of() that a real device feeds, and renders through the same page. There
// is no separate demo screen. A demonstration built from its own copy of the
// text would drift away from the product the first time either was edited, and
// would then be advertising something that does not exist.

// demoStep is one moment of the demonstration: what the fake phone looks like
// during it, and whether the mirroring screen is supposed to be up.
type demoStep struct {
	hold      time.Duration
	devices   []device.Device
	mirroring bool
}

// demoTimeline is the story the demonstration tells, and it is deliberately the
// story a first-time user actually has: plug in, get asked for permission,
// approve it, mirror. The permission step is the one that matters — it is where
// beginners give up on scrcpy, and showing Reflexo resolving it is the clearest
// possible argument for the product existing.
var demoTimeline = []demoStep{
	// Nothing connected yet: the same first screen a real visitor sees.
	{hold: 4 * time.Second},

	// The cable is in, the phone is asking permission, nobody has said yes.
	{hold: 6 * time.Second, devices: []device.Device{
		demoDevice(device.StateUnauthorized),
	}},

	// Permission granted. This is where the button becomes usable.
	{hold: 8 * time.Second, devices: []device.Device{
		demoDevice(device.StateReady),
	}},

	// Mirroring. Held longest because it is the payoff, and the only step where
	// the reader may want to take a second look.
	{hold: 12 * time.Second, devices: []device.Device{
		demoDevice(device.StateReady),
	}, mirroring: true},
}

// demoLoop is how long one pass through the timeline takes.
var demoLoop = func() time.Duration {
	var total time.Duration
	for _, s := range demoTimeline {
		total += s.hold
	}
	return total
}()

// demoDevice is the phone the demonstration pretends to have found.
//
// It is a real but unremarkable Android phone on purpose. A Samsung or a Xiaomi
// makes the guide emit vendor-specific advice, and a visitor holding neither
// would read those instructions as being about their own phone — which would
// make the demonstration worse than no demonstration at all.
func demoDevice(state device.State) device.Device {
	return device.Device{
		Serial: "DEMO0001",
		State:  state,
		Props: map[string]string{
			"ro.product.manufacturer":  "Google",
			"ro.product.brand":         "google",
			"ro.product.model":         "Pixel 7",
			"ro.build.version.release": "14",
			"ro.build.version.sdk":     "34",
		},
	}
}

// demoRun plays the timeline.
//
// The clock is injectable so the whole script can be tested without sleeping.
// That is the only reason it is a field: a test that has to wait thirty seconds
// to check four states is a test nobody runs.
type demoRun struct {
	now   func() time.Time
	start time.Time
}

func newDemoRun() *demoRun {
	now := time.Now
	return &demoRun{now: now, start: now()}
}

// moment returns the situation at this point in the demonstration.
func (d *demoRun) moment() (devices []device.Device, mirroring bool) {
	elapsed := d.now().Sub(d.start)
	if elapsed < 0 {
		elapsed = 0
	}
	// Loop forever. A demonstration that stops on a screen is a screenshot; one
	// that keeps moving is a person watching, and they will not know what to
	// do next otherwise.
	elapsed %= demoLoop

	for _, step := range demoTimeline {
		if elapsed < step.hold {
			return step.devices, step.mirroring
		}
		elapsed -= step.hold
	}

	// Unreachable: the modulo above keeps elapsed inside the timeline, and the
	// holds add up to demoLoop. Returning the last step anyway means that if
	// someone edits the timeline and breaks that invariant, the demonstration
	// shows the payoff rather than an empty screen.
	last := demoTimeline[len(demoTimeline)-1]
	return last.devices, last.mirroring
}

// showMirroring jumps straight to the mirroring step.
//
// Pressing the button has to visibly do something. Without this the click
// would appear to be ignored until the script happened to reach the last step,
// which reads as a broken button rather than a demonstration.
func (d *demoRun) showMirroring() {
	beforeLast := demoLoop - demoTimeline[len(demoTimeline)-1].hold
	d.start = d.now().Add(-beforeLast)
}

// restart returns the demonstration to its first screen, used when the user
// leaves demonstration mode by plugging in a real phone.
func (d *demoRun) restart() { d.start = d.now() }

// situation wraps the demonstration's devices for the guide, so the fake phone
// is described by exactly the code that describes a real one.
func situationFor(devices []device.Device) guide.Situation {
	return guide.Situation{Devices: devices, ADBAvailable: true}
}
