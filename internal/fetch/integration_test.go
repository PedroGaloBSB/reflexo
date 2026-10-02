//go:build integration

package fetch

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealDownload exercises the whole pipeline against the live upstream: it
// downloads the pinned scrcpy archive, verifies its SHA-256 against the signed
// manifest, extracts it and resolves the payload files.
//
// This is the test that would have caught a wrong asset name, a checksum
// algorithm change, or a layout assumption that does not hold. It is excluded
// from the default run because it needs network and ~11 MB of bandwidth.
//
//	cd C:\reflexo && go test -tags integration ./internal/fetch/ -run TestReal -v
func TestRealDownloadAndVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("pula teste de integração")
	}

	dest := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var phases []Phase

	layout, err := Ensure(ctx, dest, func(s Status) {
		phases = append(phases, s.Phase)
		t.Logf("[%s] %.0f%% %s", s.Phase, s.Percent*100, s.Message)
	})
	if err != nil {
		t.Fatalf("Ensure falhou: %v", err)
	}

	if layout.Binary == "" {
		t.Fatal("executável do scrcpy não foi localizado")
	}
	if _, err := os.Stat(layout.Binary); err != nil {
		t.Errorf("executável inexistente em %s: %v", layout.Binary, err)
	}
	if _, err := os.Stat(layout.Server); err != nil {
		t.Errorf("scrcpy-server inexistente em %s: %v", layout.Server, err)
	}

	t.Logf("binário: %s", layout.Binary)
	t.Logf("servidor: %s", layout.Server)
	t.Logf("adb: %s", layout.ADB)
	t.Logf("ícones: %s", layout.IconDir)

	if layout.ADB == "" {
		t.Error("adb não foi localizado no pacote")
	}

	// The archive must have been downloaded, verified and extracted in order.
	if len(phases) < 4 {
		t.Errorf("fases observadas: %v, esperava download, verificação e extração", phases)
	}

	// Second call must hit the cache and skip the network entirely.
	cached, err := Ensure(ctx, dest, func(s Status) { phases = append(phases, s.Phase) })
	if err != nil {
		t.Fatalf("segunda chamada falhou: %v", err)
	}
	if cached.Binary != layout.Binary {
		t.Errorf("caminho mudou na segunda chamada: %q → %q", layout.Binary, cached.Binary)
	}
}

// TestRealChecksumManifest verifies the parser against the live manifest and
// confirms the digests agree with the ones the scrcpy project documents.
func TestRealChecksumManifest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sums, err := fetchChecksums(ctx)
	if err != nil {
		t.Fatalf("não foi possível obter o manifesto: %v", err)
	}

	if len(sums) == 0 {
		t.Fatal("manifesto veio vazio")
	}

	// Cross-check against doc/build.md in the scrcpy repository.
	want := map[string]string{
		"scrcpy-server-v4.1":    "deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae",
		"scrcpy-win64-v4.1.zip": "5b12172b3264b2889f4583ee64752ce832e29bc8b1089dca81093459697165db",
		"scrcpy-win32-v4.1.zip": "fa57b36622a53b6aec74c5e5b5c08236165efa445c4f186d48f176ebf9c24eec",
	}

	for name, digest := range want {
		got, ok := sums[name]
		if !ok {
			t.Errorf("%s ausente do manifesto", name)
			continue
		}
		if got != digest {
			t.Errorf("%s: manifesto tem %s, documentação tem %s", name, got, digest)
		}
	}

	t.Logf("manifesto contém %d entradas", len(sums))
}
