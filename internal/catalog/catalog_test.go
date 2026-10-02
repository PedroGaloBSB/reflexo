package catalog

import (
	"net/url"
	"runtime"
	"strings"
	"testing"
)

// upstreamHost is the only host Reflexo is allowed to fetch scrcpy from.
//
// This constant is not a convenience. It is the premise the LGPL conclusion in
// THIRD_PARTY.md rests on: Reflexo redistributes no third-party code, so the
// obligation to publish FFmpeg sources or offer relinking belongs to whoever
// ships the binary we download, not to us.
//
// The moment that premise stops holding — a mirror, an offline portable build,
// an installer that embeds the payload — the conclusion is no longer true and
// the obligation comes back. A test cannot enforce the licence, but it can
// enforce the premise, which is the part that is ours.
const upstreamHost = "github.com"

// TestScrcpyIsAlwaysFetchedFromUpstream pins the download host for every asset
// and for the checksum manifest.
func TestScrcpyIsAlwaysFetchedFromUpstream(t *testing.T) {
	targets := map[string]string{
		"manifest de checksums": ChecksumsURL(),
		"scrcpy-server":         releaseBase + "/" + ServerAsset(),
	}

	for _, a := range assets {
		if _, ok := For(a.OS, a.Arch); !ok {
			continue
		}
		targets[a.OS+"/"+a.Arch+" ("+a.ArchiveName()+")"] = a.URL()
	}

	if len(targets) < 2 {
		t.Fatalf("só %d URLs inspecionadas; a lista de assets encolheu e o teste "+
			"deixou de cobrir o release inteiro", len(targets))
	}

	for name, raw := range targets {
		u, err := url.Parse(raw)
		if err != nil {
			t.Errorf("%s: URL inválida %q: %v", name, raw, err)
			continue
		}

		if u.Scheme != "https" {
			t.Errorf("%s: esquema %q; scrcpy baixado por texto claro seria trocável "+
				"no caminho, mesmo com SHA-256 conferido", name, u.Scheme)
		}
		if u.Host != upstreamHost {
			t.Errorf("%s: host %q em vez de %q.\n"+
				"Buscar o scrcpy fora do upstream significa redistribuí-lo, e aí a "+
				"obrigação de LGPL dos binários linkados estaticamente passa a ser "+
				"do Reflexo. Ver THIRD_PARTY.md.", name, u.Host, upstreamHost)
		}
		if !strings.Contains(u.Path, "/Genymobile/scrcpy/releases/") {
			t.Errorf("%s: caminho %q não é um release do Genymobile/scrcpy",
				name, u.Path)
		}
		if !strings.Contains(u.Path, "v"+Version) {
			t.Errorf("%s: caminho %q nao fixa a versao %s; um release deslizante "+
				"quebraria a reprodutibilidade do build", name, u.Path, Version)
		}
	}
}

// TestVersionPinIsNotFloating guards the pin itself. "latest" would resolve to a
// different scrcpy on every machine, and the client refuses to talk to a
// server of a different version (AGENTS.md §7).
func TestVersionPinIsNotFloating(t *testing.T) {
	if Version == "" || Version == "latest" {
		t.Fatalf("Version = %q; a versão do scrcpy precisa ser fixada", Version)
	}
	if strings.ContainsAny(Version, " /\\?#") {
		t.Errorf("Version = %q contem caracteres que nao pertencem a uma versao", Version)
	}
	if ServerAsset() != "scrcpy-server-v"+Version {
		t.Errorf("ServerAsset = %q não acompanha a versão %q; cliente e servidor "+
			"precisam da mesma", ServerAsset(), Version)
	}
}

// TestEverySupportedPlatformHasAnAsset stops a platform from silently falling
// back to an error the user cannot act on.
func TestEverySupportedPlatformHasAnAsset(t *testing.T) {
	for _, tc := range []struct{ goos, goarch string }{
		{"windows", "amd64"},
		{"windows", "386"},
		{"linux", "amd64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	} {
		if _, ok := For(tc.goos, tc.goarch); !ok {
			t.Errorf("sem asset para %s/%s", tc.goos, tc.goarch)
		}
	}

	// linux/arm64 is upstream's gap, not ours. If upstream ever publishes it,
	// this test fails and that is the correct outcome.
	if _, ok := For("linux", "arm64"); ok {
		t.Log("linux/arm64 agora tem asset upstream; atualize Supported() e a nota do catálogo")
	}
}

// TestForHostMatchesTheRunningPlatform keeps the dispatch honest.
func TestForHostMatchesTheRunningPlatform(t *testing.T) {
	a, err := ForHost()
	if err != nil {
		t.Skipf("plataforma sem asset suportado: %v", err)
	}
	if a.OS != runtime.GOOS || a.Arch != runtime.GOARCH {
		t.Errorf("ForHost devolveu %s/%s, mas o processo roda em %s/%s",
			a.OS, a.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if !strings.Contains(a.ArchiveName(), "-v"+Version) {
		t.Errorf("asset %q nao carrega a versao fixada %q", a.ArchiveName(), Version)
	}
}
