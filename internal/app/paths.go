package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// payloadDirName is the folder Reflexo creates inside the platform base
// directory. Lowercase on purpose, including on macOS: the capitalised
// "Reflexo" convention exists to look right in Finder, and matching the log
// directory is worth more than that.
const payloadDirName = "reflexo"

// DataDir returns the per-user directory where Reflexo stores the scrcpy
// payload — the downloaded, verified binaries and the extracted tree.
//
// Two rules decide this, and both come from the environment the product
// targets.
//
// **Never administrator rights.** Every candidate is inside the user's own
// profile, so the caller can write it without elevation (AGENTS.md §4).
//
// **Never a cloud-synced directory.** os.UserConfigDir() returns %AppData% on
// Windows, which a corporate profile or OneDrive known-folder-move replicates
// off the machine. The payload is twenty-five megabytes: it would sync slowly,
// it would burn the user's quota, and on a metered link it would make the
// first launch miserable. This is the same reasoning that puts the log in
// LOCALAPPDATA, with more force — the log is a few kilobytes of text; this is a
// directory tree.
//
// Per platform:
//
//   - Windows: LOCALAPPDATA, via os.UserCacheDir(). Never Roaming.
//   - macOS:   ~/Library/Application Support. Not synced by default, and unlike
//     ~/Library/Caches it is never silently evicted by the OS — being evicted
//     would mean re-downloading twenty-five megabytes for no reason.
//   - Linux:   $XDG_DATA_HOME, falling back to ~/.local/share, which is where
//     the XDG Base Directory specification puts application data. Config
//     (~/.config) is for settings the user edits; a downloaded binary payload
//     is not that.
func DataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		base, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("não foi possível localizar o LOCALAPPDATA: %w", err)
		}
		return filepath.Join(base, payloadDirName), nil

	case "darwin":
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("não foi possível localizar a pasta de suporte do usuário: %w", err)
		}
		return filepath.Join(base, payloadDirName), nil

	default:
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, payloadDirName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("não foi possível localizar a pasta do usuário: %w", err)
		}
		return filepath.Join(home, ".local", "share", payloadDirName), nil
	}
}

// LegacyDataDir returns where Reflexo used to keep the payload, or an empty
// string when there is nothing to migrate from.
//
// Only Windows ever had one. os.UserConfigDir() resolves to %AppData% there and
// to a sensible location everywhere else, so the move was only ever necessary on
// Windows — which means only Windows needs to carry the old path around.
func LegacyDataDir() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	legacy := filepath.Join(base, payloadDirName)

	// Only report a legacy directory that actually exists. Reporting one that
	// does not would make every fresh install report a migration.
	if _, err := os.Stat(legacy); err != nil {
		return ""
	}
	return legacy
}

// MigratePayload moves a payload left in the legacy directory into dest.
//
// legacy is passed in rather than read inside, so the move itself can be tested
// without a machine that happens to have an old installation lying around —
// and so the platform policy stays in one place.
//
// It is a convenience and never a requirement. If anything goes wrong the
// caller simply downloads the payload again: re-downloading costs twenty-five
// megabytes once, whereas failing to start would cost the user the product.
//
// The move is a rename, which is atomic and instant when both paths are on the
// same volume. When it is not — LOCALAPPDATA redirected to a network share
// while Roaming stayed local — it is left alone rather than copied and deleted,
// because deleting from a directory Reflexo no longer manages is not a decision
// this function gets to make.
func MigratePayload(dest, legacy string) (bool, error) {
	if legacy == "" || dest == legacy {
		return false, nil
	}

	entries, err := os.ReadDir(legacy)
	if err != nil {
		// Nothing to migrate, or not readable. Either way this is not fatal.
		return false, nil //nolint:nilerr // absence of a legacy dir is the normal case
	}

	// Only the payload subtrees move. Anything else the old directory might
	// contain belongs to the user and stays where they put it.
	var names []string
	for _, e := range entries {
		if e.IsDir() && isPayloadDir(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return false, nil
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return false, fmt.Errorf("não foi possível criar %s: %w", dest, err)
	}

	moved := false
	for _, name := range names {
		src := filepath.Join(legacy, name)
		dst := filepath.Join(dest, name)

		// Never overwrite something already in place. A previous migration, or
		// a download that finished while this was being written, wins.
		if _, err := os.Stat(dst); err == nil {
			continue
		}

		if err := os.Rename(src, dst); err != nil {
			return moved, fmt.Errorf(
				"não foi possível mover o scrcpy de %s para %s: %w",
				src, dst, err)
		}
		moved = true
	}

	return moved, nil
}

// isPayloadDir reports whether a directory inside the data directory is a
// downloaded scrcpy payload rather than something else.
//
// The name is the whole test. fetch.Ensure extracts the upstream archive, whose
// top-level directory is always "scrcpy-<platform>-<version>", and nothing else
// is ever written there. Matching on the prefix means a folder the user put
// there by hand is never moved, renamed or deleted by a migration.
func isPayloadDir(name string) bool {
	return strings.HasPrefix(name, "scrcpy-")
}

// PayloadIsPresent reports whether dest already holds an extracted payload.
//
// Used by the tests to assert that a migration actually moved something, and
// available so a caller can report "already installed" without paying for the
// full Ensure().
func PayloadIsPresent(dest string) bool {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && isPayloadDir(e.Name()) {
			return true
		}
	}
	return false
}
