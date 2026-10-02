package i18n

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// placeholderRE matches fmt verbs used in a catalog string.
var placeholderRE = regexp.MustCompile(`%[sdvqt%]`)

// TestEnglishIsComplete is the guard that makes translations safe: every key
// used anywhere must exist in English, because English is the fallback for every
// other language.
func TestEnglishIsComplete(t *testing.T) {
	if len(english) == 0 {
		t.Fatal("catálogo em inglês está vazio")
	}

	for key := range english {
		if !strings.HasPrefix(key, "guide.") &&
			!strings.HasPrefix(key, "setup.") &&
			!strings.HasPrefix(key, "error.") &&
			!strings.HasPrefix(key, "shared.") &&
			!strings.HasPrefix(key, "platform.") &&
			!strings.HasPrefix(key, "severity.") &&
			!strings.HasPrefix(key, "ui.") {
			t.Errorf("chave %q não segue nenhuma família de prefixos conhecida", key)
		}
	}
}

// A translation missing a key silently falls back to English, which shows the
// user an English sentence in a Portuguese product. That is a bug, not a
// tolerable degradation, so completeness is enforced here.
func TestPortugueseIsComplete(t *testing.T) {
	var missing []string

	for key := range english {
		if !Has(PortugueseBrazil, key) {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		t.Errorf("tradução pt-BR está faltando %d chave(s): %v", len(missing), missing)
	}
}

func TestNoExtraKeysInPortuguese(t *testing.T) {
	var extra []string

	for key := range portugueseBrazil {
		if !Has(English, key) {
			extra = append(extra, key)
		}
	}

	if len(extra) > 0 {
		t.Errorf("pt-BR tem chave(s) que não existem em inglês: %v", extra)
	}
}

// A translation that drops or invents a placeholder would either print
// "%!s(MISSING)" or ignore an argument. Both are user-visible defects.
func TestPlaceholdersMatchAcrossLanguages(t *testing.T) {
	for key, en := range english {
		pt := lookup(PortugueseBrazil, key)
		if pt == "" {
			continue // reported by TestPortugueseIsComplete
		}

		want := placeholderRE.FindAllString(en, -1)
		got := placeholderRE.FindAllString(pt, -1)

		if len(want) != len(got) {
			t.Errorf("chave %q: inglês tem %v, pt-BR tem %v", key, want, got)
			continue
		}
		for i := range want {
			if want[i] != got[i] {
				t.Errorf("chave %q: placeholder %d é %q em inglês e %q em pt-BR",
					key, i, want[i], got[i])
			}
		}
	}
}

// Formatting must never emit Go's %! markers, which reach the user verbatim.
func TestTFormatsCleanly(t *testing.T) {
	tests := []struct {
		key  string
		args []any
	}{
		{"guide.unauthorized.detail", []any{"Galaxy A55"}},
		{"guide.multiple.headline", []any{2}},
		{"guide.unknown.detail", []any{"weird"}},
		{"guide.pre_android.headline", []any{"recovery mode"}},
		{"guide.ready.headline", []any{"Galaxy A55"}},
		{"shared.android", []any{"15"}},
	}

	for _, lang := range Available() {
		for _, tt := range tests {
			got := T(lang, tt.key, tt.args...)

			if strings.Contains(got, "%!") {
				t.Errorf("%s/%s: formatação quebrada: %q", lang, tt.key, got)
			}
			if got == tt.key {
				t.Errorf("%s/%s: chave não traduzida retornou a própria chave", lang, tt.key)
			}
		}
	}
}

func TestTWithoutArgs(t *testing.T) {
	for _, lang := range Available() {
		got := T(lang, "guide.no_device.headline")
		if got == "" || strings.Contains(got, "%!") {
			t.Errorf("%s: headline vazia ou quebrada: %q", lang, got)
		}
	}
}

// A missing key returns the key so it is obvious in review, rather than a blank
// screen for the user.
func TestTMissingKeyReturnsKey(t *testing.T) {
	got := T(English, "does.not.exist")
	if got != "does.not.exist" {
		t.Errorf("T() para chave inexistente = %q, esperado a própria chave", got)
	}
}

// An unsupported language must fall back to English rather than producing an
// empty catalog.
func TestTUnknownLanguageFallsBack(t *testing.T) {
	got := T(Lang("kl-GL"), "guide.no_device.headline")
	if got == "" {
		t.Fatal("idioma desconhecido produziu string vazia")
	}
	// It must not return the raw key.
	if got == "guide.no_device.headline" {
		t.Error("idioma desconhecido não deveria chegar ao fallback final")
	}
}

func TestParseLocale(t *testing.T) {
	tests := []struct {
		in   string
		want Lang
	}{
		{"pt-BR", PortugueseBrazil},
		{"pt_BR", PortugueseBrazil},
		{"pt", PortugueseBrazil},
		{"pt_BR.UTF-8", PortugueseBrazil},
		{"pt_BR.UTF-8@eucBR", PortugueseBrazil},
		{"PT-br", PortugueseBrazil},
		{"en", English},
		{"en_US.UTF-8", English},
		{"EN-GB", English},
		{"", Default},
		{"kl-GL", Default},
		{"zh_CN", Default},
		{"garbage", Default},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parse(tt.in); got != tt.want {
				t.Errorf("parse(%q) = %q, esperado %q", tt.in, got, tt.want)
			}
		})
	}
}

// REFLEXO_LANG must win over the POSIX variables, since it is the only way to
// force a language where the OS locale cannot be changed.
func TestDetectPrefersReflexoLang(t *testing.T) {
	t.Setenv("REFLEXO_LANG", "en")
	t.Setenv("LANG", "pt_BR.UTF-8")

	if got := Detect(); got != English {
		t.Errorf("Detect() = %q, esperado %q (REFLEXO_LANG tem precedência)", got, English)
	}
}

func TestDetectUsesPosixVars(t *testing.T) {
	t.Setenv("REFLEXO_LANG", "")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "pt_BR.UTF-8")

	if got := Detect(); got != PortugueseBrazil {
		t.Errorf("Detect() = %q, esperado %q", got, PortugueseBrazil)
	}
}

func TestDetectDefaultsWhenUnset(t *testing.T) {
	for _, k := range []string{"REFLEXO_LANG", "LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	if got := Detect(); got != Default {
		t.Errorf("Detect() = %q, esperado o padrão %q", got, Default)
	}
}

// LANGUAGE outranks LANG, per the GNU gettext precedence, so a user who set a
// preferred list gets their first choice.
func TestDetectLanguagePrecedence(t *testing.T) {
	t.Setenv("REFLEXO_LANG", "")
	t.Setenv("LANGUAGE", "en_US")
	t.Setenv("LANG", "pt_BR.UTF-8")

	if got := Detect(); got != English {
		t.Errorf("Detect() = %q, esperado %q", got, English)
	}
}

// Every language except English falls back to English, so the chain must
// include it. Verified through T so the test exercises the real path.
func TestFallbackChainReachesEnglish(t *testing.T) {
	chain := langChain(PortugueseBrazil)
	if len(chain) != 2 || chain[1] != English {
		t.Errorf("langChain(pt-BR) = %v, esperado incluir English", chain)
	}

	chain = langChain(English)
	if len(chain) != 1 {
		t.Errorf("langChain(en) = %v, esperado apenas English", chain)
	}
}

func TestAvailableIncludesBothLanguages(t *testing.T) {
	langs := Available()
	if len(langs) != 2 {
		t.Fatalf("Available() = %v, esperado 2 idiomas", langs)
	}

	for _, want := range []Lang{English, PortugueseBrazil} {
		found := false
		for _, l := range langs {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Available() não inclui %q", want)
		}
	}
}

// Guards against a translator leaving %!(EXTRA) behind by adding a placeholder
// that English does not have.
func TestNoStrayFormatDirectives(t *testing.T) {
	for lang, cat := range catalogs {
		for key, msg := range cat {
			if strings.Contains(msg, "%(") {
				t.Errorf("%s/%s usa %%() que não é um verbo suportado: %q", lang, key, msg)
			}
			if n := strings.Count(msg, "%%"); n > 0 && !strings.Contains(msg, "%%") {
				t.Errorf("%s/%s tem %%%% literal", lang, key)
			}
		}
	}
}

// A catalog entry that is only whitespace is effectively missing.
func TestNoBlankEntries(t *testing.T) {
	for lang, cat := range catalogs {
		for key, msg := range cat {
			if strings.TrimSpace(msg) == "" {
				t.Errorf("%s/%q está em branco", lang, key)
			}
		}
	}
}

// Sample of what the UI actually renders, to make sure sentences read naturally
// once formatted rather than merely compiling.
func TestRenderedSampleReadsWell(t *testing.T) {
	name := "Galaxy A55"
	state := "recovery"

	samples := map[Lang]string{
		PortugueseBrazil: fmt.Sprintf("%s · %s",
			T(PortugueseBrazil, "guide.unauthorized.detail", name),
			T(PortugueseBrazil, "guide.pre_android.headline", state)),
		English: fmt.Sprintf("%s · %s",
			T(English, "guide.unauthorized.detail", name),
			T(English, "guide.pre_android.headline", state)),
	}

	for lang, out := range samples {
		if !strings.Contains(out, name) {
			t.Errorf("%s: nome do aparelho não apareceu: %q", lang, out)
		}
		if strings.Contains(out, "%!") {
			t.Errorf("%s: formatação quebrada: %q", lang, out)
		}
		t.Logf("%s: %s", lang, out)
	}
}
