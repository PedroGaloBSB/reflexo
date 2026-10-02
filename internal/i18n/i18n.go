// Package i18n holds the user-facing strings, keyed so they can be translated
// without touching control flow.
//
// Design notes:
//
//   - English is the source language. Every key must exist in English; a
//     translation that is missing a key falls back to English rather than
//     showing a raw key to the user.
//   - Strings are looked up by key and formatted with fmt verbs. Keys are
//     structured like "guide.no_device.headline", not "noDeviceHeadline", so
//     the catalog reads as a list rather than as code.
//   - The language is chosen once at startup and never changes. A UI that
//     switches language while the user is reading it is worse than one that
//     picked the wrong language initially.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang is a supported language code.
type Lang string

const (
	// English is the source language.
	English Lang = "en"
	// PortugueseBrazil is the primary target, matching the product's origin.
	PortugueseBrazil Lang = "pt-BR"
)

// Default is used when detection is inconclusive.
//
// Portuguese is the default because every string was written in Portuguese
// first and the primary audience is Brazilian; English is the fallback for any
// key that has not been translated yet.
const Default = PortugueseBrazil

// available lists every language Reflexo ships.
var available = []Lang{English, PortugueseBrazil}

// Available returns the supported languages.
func Available() []Lang { return available }

// Catalog maps keys to translated strings for one language.
type Catalog map[string]string

// catalogs holds every translation.
var catalogs = map[Lang]Catalog{
	English:          english,
	PortugueseBrazil: portugueseBrazil,
}

// Detect chooses a language from the environment.
//
// Precedence, highest first:
//
//  1. REFLEXO_LANG, which exists for testing and for support cases where the
//     operating system locale cannot be changed.
//  2. LANGUAGE, then LC_ALL, then LC_MESSAGES, then LANG — the POSIX variables.
//  3. Default.
//
// Windows does not set the POSIX variables, so a Portuguese Windows user gets
// Default. That is the right outcome here only because Default is Portuguese;
// if the default ever changes, this comment must change with it.
func Detect() Lang {
	if v := strings.TrimSpace(os.Getenv("REFLEXO_LANG")); v != "" {
		return parse(v)
	}

	for _, key := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return parse(v)
		}
	}

	return Default
}

// parse maps a locale string onto a supported language.
//
// "pt", "pt_BR", "pt-BR.UTF-8" and "pt_BR.UTF-8@eucBR" all resolve to
// Portuguese; anything unknown resolves to the Default rather than erroring,
// because a user with an exotic locale should still get a working product.
func parse(v string) Lang {
	// Drop encoding and modifier: pt_BR.UTF-8@eucBR -> pt_BR
	if i := strings.IndexAny(v, ".@"); i >= 0 {
		v = v[:i]
	}
	v = strings.ReplaceAll(v, "-", "_")

	lower := strings.ToLower(v)

	switch {
	case strings.HasPrefix(lower, "pt_br"):
		return PortugueseBrazil
	case strings.HasPrefix(lower, "pt"):
		return PortugueseBrazil
	case strings.HasPrefix(lower, "en"):
		return English
	default:
		return Default
	}
}

// T translates a key in the given language, falling back to English and then to
// the key itself.
//
// The final fallback returns the key rather than an empty string so that a
// missing translation is visible during review instead of silently blank.
func T(lang Lang, key string, args ...any) string {
	msg := lookup(lang, key)
	if msg == "" {
		msg = lookup(English, key)
	}
	if msg == "" {
		return key
	}

	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// lookup finds a key in one catalog without falling back.
func lookup(lang Lang, key string) string {
	if c, ok := catalogs[lang]; ok {
		return c[key]
	}
	return ""
}

// Has reports whether a key exists in the given language. Tests use it to keep
// translations complete.
func Has(lang Lang, key string) bool { return lookup(lang, key) != "" }

// uiKeys is the set of strings the browser renders itself, as opposed to the
// instruction text the Go side builds.
//
// The browser cannot import a Go package, so these are shipped to the page as
// part of the state payload and applied through data-i18n attributes. Listing
// them explicitly — rather than shipping the whole catalog — keeps the payload
// small and means a guide string can never be quietly ignored by the UI.
//
// TestUIStringsCoverMarkup asserts this list matches the data-i18n attributes
// actually present in web/index.html and the keys app.js asks for.
var uiKeys = []string{
	"setup.preparing",
	"setup.preparing_detail",
	"setup.failed_title",
	"setup.note",

	"severity.idle",
	"severity.action",
	"severity.warn",
	"severity.ready",

	"ui.connecting",
	"ui.start",
	"ui.running",
	"ui.close_to_stop",
	"ui.unavailable",
	"ui.conn_lost",
	"ui.start_failed",
	"ui.start_conn_failed",
	"ui.attribution_prefix",
	"ui.attribution_suffix",
	"ui.non_affiliation",
	"ui.demo_banner",
	"ui.demo_try",
	"ui.demo_stop",
}

// UIKeys returns the keys handed to the web UI.
func UIKeys() []string { return append([]string(nil), uiKeys...) }

// UIStrings resolves the browser-facing strings for one language.
//
// It is a plain map rather than a slice because the page looks strings up by
// key, and because it makes a missing key impossible to index by accident.
func UIStrings(lang Lang) map[string]string {
	out := make(map[string]string, len(uiKeys))
	for _, k := range uiKeys {
		out[k] = T(lang, k)
	}
	return out
}

// langChain returns the lookup order for a language: the language itself, then
// English. Exposed for tests so completeness can be asserted without reaching
// into unexported state.
func langChain(lang Lang) []Lang {
	if lang == English {
		return []Lang{English}
	}
	return []Lang{lang, English}
}
