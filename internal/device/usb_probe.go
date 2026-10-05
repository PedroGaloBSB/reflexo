package device

// UsbAndroidState is not implemented yet, on any platform, and it returns
// UsbUnknown rather than a guess.
//
// That is deliberate and it is the whole point of having three states. This
// function exists to stop Reflexo from confidently telling someone that no phone
// is connected while their phone is sitting on the desk. Returning a guess would
// reintroduce exactly that failure in a new place: on Linux the obvious
// implementation reads /sys/bus/usb/devices/*/idVendor and concludes "a USB
// device is present, so it must be a phone", which fires the moment anyone
// plugs in a mouse. A wrong message is worse than the honest silence.
//
// What was learned trying to build it on Windows, recorded so the work is not
// lost:
//
//   - Enumerating the ADB interface by SetupAPI class returns nothing. Windows
//     registers it under the generic USBDevice class
//     {88bae032-5a81-49f0-bc3d-a4ff138216d6}, and "ADB Interface" is only a
//     friendly name installed by the driver.
//   - The marker that does work is the compatible id
//     "Class_ff&SubClass_42&Prot_01" — USB class 0xFF, subclass 0x42, protocol
//     0x01, the Android Debug Bridge interface. Reading SPDRP_COMPATID through
//     SetupDiGetDeviceRegistryProperty and matching that signature identifies it
//     reliably, and a real SM-A556E was confirmed carrying it.
//   - Separating "phone unplugged" from "phone with debugging off" then needs
//     the parent device. DEVPKEY_Device_Parent gives the USB composite device,
//     which stays present while the cable is plugged. That check is untested:
//     on the machine that produced these notes the composite instance reported
//     itself absent while its own children reported themselves present, which
//     needs understanding before it can be trusted.
//
// It is not wired into App.Refresh, so nothing depends on it yet. ADR-0007
// therefore remains open, and the guide still shows the old message.
func UsbAndroidState() UsbState { return UsbUnknown }
