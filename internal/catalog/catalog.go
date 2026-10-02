// Package catalog pins which upstream scrcpy build Reflexo downloads.
//
// Per AGENTS.md §3 the scrcpy is a runtime dependency, never a vendored fork.
// This file is the single place where its version is pinned: bumping scrcpy
// must be a change here and nowhere else in the tree.
package catalog

import (
	"fmt"
	"runtime"
)

// Version is the pinned upstream scrcpy version.
//
// The scrcpy client and scrcpy-server refuse to talk to each other when their
// versions differ (AGENTS.md §7), so this constant also fixes the server that
// gets pushed to the device. Bump them together, never separately.
const Version = "4.1"

// Archive kinds used by the upstream releases.
const (
	ArchiveZip   = "zip"
	ArchiveTarGz = "tar.gz"
)

// releaseBase is the GitHub release download root. "latest" cannot be used:
// Reflexo needs a reproducible, pinnable artifact.
const releaseBase = "https://github.com/Genymobile/scrcpy/releases/download/v" + Version

// serverAsset is the platform-independent server pushed to the device.
const serverAsset = "scrcpy-server-v" + Version

// Asset is one downloadable upstream artifact for a given platform.
type Asset struct {
	OS   string // runtime.GOOS: windows, linux, darwin
	Arch string // runtime.GOARCH: amd64, 386, arm64
	// Stem is the asset name without the version suffix or extension.
	Stem string
	// Kind is ArchiveZip or ArchiveTarGz.
	Kind string
	// Binary is the executable name inside the extracted archive.
	Binary string
	// Subdir is the path inside the archive holding the payload, relative to
	// the extraction root. Empty when the archive is flat.
	Subdir string
}

// assets mirrors the naming used by the upstream Genymobile/scrcpy releases.
//
// Note the deliberate absence of linux/arm64: upstream does not publish a
// Linux aarch64 build, so Reflexo cannot offer one either. Supporting it would
// mean compiling scrcpy ourselves, which AGENTS.md §3 forbids.
var assets = []Asset{
	{
		OS: "windows", Arch: "amd64", Stem: "scrcpy-win64",
		Kind: ArchiveZip, Binary: "scrcpy.exe",
	},
	{
		OS: "windows", Arch: "386", Stem: "scrcpy-win32",
		Kind: ArchiveZip, Binary: "scrcpy.exe",
	},
	{
		OS: "linux", Arch: "amd64", Stem: "scrcpy-linux-x86_64",
		Kind: ArchiveTarGz, Binary: "scrcpy",
	},
	{
		OS: "darwin", Arch: "amd64", Stem: "scrcpy-macos-x86_64",
		Kind: ArchiveTarGz, Binary: "scrcpy", Subdir: "scrcpy.app/Contents/MacOS",
	},
	{
		OS: "darwin", Arch: "arm64", Stem: "scrcpy-macos-aarch64",
		Kind: ArchiveTarGz, Binary: "scrcpy", Subdir: "scrcpy.app/Contents/MacOS",
	},
}

// ServerAsset is the scrcpy-server file pushed to the device. It is
// architecture-independent and ships with every client archive.
func ServerAsset() string { return serverAsset }

// ArchiveName is the full asset filename, e.g. "scrcpy-win64-v4.1.zip".
//
// The "v" prefix before the version is part of the upstream naming and is easy
// to get wrong; catalog_test.go pins the exact strings so a typo here cannot
// reach production, where it would surface only as a failed download.
func (a Asset) ArchiveName() string {
	return fmt.Sprintf("%s-v%s.%s", a.Stem, Version, a.Kind)
}

// URL is the absolute download location of the archive.
func (a Asset) URL() string { return releaseBase + "/" + a.ArchiveName() }

// ChecksumsURL is the upstream signed manifest of every asset digest. Reflexo
// verifies downloads against it rather than trusting the transport.
func ChecksumsURL() string { return releaseBase + "/SHA256SUMS.txt" }

// For returns the asset matching the running platform.
//
// The boolean is false when upstream publishes no build for this platform,
// which is currently the case for every non-amd64 Linux host (AGENTS.md §5).
func For(goos, goarch string) (Asset, bool) {
	for _, a := range assets {
		if a.OS == goos && a.Arch == goarch {
			return a, true
		}
	}
	return Asset{}, false
}

// ForHost is For for the machine Reflexo is running on.
func ForHost() (Asset, error) {
	a, ok := For(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return Asset{}, fmt.Errorf(
			"scrcpy %s não publica binários para %s/%s; plataformas suportadas: windows/amd64, windows/386, linux/amd64, darwin/amd64, darwin/arm64",
			Version, runtime.GOOS, runtime.GOARCH,
		)
	}
	return a, nil
}

// Supported reports whether the running platform has an upstream build.
func Supported() bool {
	_, ok := For(runtime.GOOS, runtime.GOARCH)
	return ok
}

// SupportedList renders the supported platform matrix for the error message in
// the manual.
func SupportedList() string {
	const sep = ", "
	out := ""
	for i, a := range assets {
		if i > 0 {
			out += sep
		}
		out += a.OS + "/" + a.Arch
	}
	return out
}
