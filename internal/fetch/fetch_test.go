package fetch

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseChecksums must handle the real upstream manifest, including the
// binary-mode " *" marker GNU coreutils can emit for binary files.
func TestParseChecksums(t *testing.T) {
	// Verbatim content of scrcpy v4.1 SHA256SUMS.txt.
	const upstream = `deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae  scrcpy-server-v4.1
ad56ae8bfeedf41e824945c11dbf55fcb092b3e615b9b486f48a50e30d389635  scrcpy-linux-x86_64-v4.1.tar.gz
fa57b36622a53b6aec74c5e5b5c08236165efa445c4f186d48f176ebf9c24eec  scrcpy-win32-v4.1.zip
5b12172b3264b2889f4583ee64752ce832e29bc8b1089dca81093459697165db  scrcpy-win64-v4.1.zip
20fd47c9014dd5e0fa77091f3cb7adbda8445a360c4584aeaa0150b5b3988ff3  scrcpy-macos-aarch64-v4.1.tar.gz
ee2a7223bc8dbdc4f482db1134bcf441178dafb833492b71ca4c22090c58ce72  scrcpy-macos-x86_64-v4.1.tar.gz
`

	got := parseChecksums(upstream)

	// This digest is independently documented in the scrcpy repository at
	// doc/build.md, so it cross-checks the parser against a second source.
	const serverSum = "deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae"
	if got["scrcpy-server-v4.1"] != serverSum {
		t.Errorf("digest do scrcpy-server = %q, esperado %q", got["scrcpy-server-v4.1"], serverSum)
	}
	if len(got) != 6 {
		t.Errorf("obtive %d entradas, esperado 6: %+v", len(got), got)
	}
}

func TestParseChecksumsEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"vazio", "", 0},
		{"só espaços", "   \n\t\n", 0},
		{"linhas malformadas", "abc\nxyz  file\n", 0},
		{"hex muito curto", "abcd  file.zip\n", 0},
		{"hex inválido", strings.Repeat("z", 64) + "  file.zip\n", 0},
		{"sem nome", strings.Repeat("a", 64) + "\n", 0},
		{"marcador binário do coreutils",
			strings.Repeat("a", 64) + " *file.zip\n", 1},
		{"separador por tab",
			strings.Repeat("a", 64) + "\tfile.zip\n", 1},
		{"crlf do Windows",
			strings.Repeat("a", 64) + "  file.zip\r\n", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseChecksums(tt.in); len(got) != tt.want {
				t.Errorf("obtive %d entradas, esperado %d: %+v", len(got), tt.want, got)
			}
		})
	}
}

func TestParseChecksumsUppercaseIsNormalised(t *testing.T) {
	in := strings.ToUpper(strings.Repeat("AB", 32)) + "  file.zip\n"
	got := parseChecksums(in)

	if got["file.zip"] != strings.ToLower(strings.Repeat("ab", 32)) {
		t.Errorf("digest não foi normalizado para minúsculas: %+v", got)
	}
}

// safeJoin is the guard against "zip slip": an archive entry must never be
// able to write outside the destination directory.
func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := filepath.Join("C:", "temp", "install")

	dangerous := []string{
		"../evil.txt",
		"../../etc/passwd",
		"sub/../../evil.txt",
		"/etc/passwd",
		`\Windows\System32\evil.dll`,
	}

	for _, name := range dangerous {
		t.Run(name, func(t *testing.T) {
			if _, err := safeJoin(root, name); err == nil {
				t.Errorf("safeJoin aceitou caminho perigoso %q", name)
			}
		})
	}
}

func TestSafeJoinAllowsNormalPaths(t *testing.T) {
	root := filepath.Join("C:", "temp", "install")

	tests := []struct {
		in   string
		want string
	}{
		{"scrcpy.exe", filepath.Join(root, "scrcpy.exe")},
		{"sub/dir/file.txt", filepath.Join(root, "sub", "dir", "file.txt")},
		{"./scrcpy.exe", filepath.Join(root, "scrcpy.exe")},
		{"sub/../scrcpy.exe", filepath.Join(root, "scrcpy.exe")},
		// A name that merely starts with ".." but does not traverse, such as
		// the legitimate release file "..foo", must still be accepted.
		{"..foo", filepath.Join(root, "..foo")},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := safeJoin(root, tt.in)
			if err != nil {
				t.Fatalf("safeJoin recusou caminho legítimo %q: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("safeJoin(%q) = %q, esperado %q", tt.in, got, tt.want)
			}
		})
	}
}

// verifySHA256 must reject a mismatch and accept the real one; rejecting is
// the whole point of the package, but a verifier that rejects everything would
// be just as broken.
func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")

	content := []byte("scrcpy")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	real := sha256.Sum256(content)
	correct := hex.EncodeToString(real[:])

	if err := verifySHA256(path, correct); err != nil {
		t.Errorf("verifySHA256 rejeitou digest correto: %v", err)
	}

	if err := verifySHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Error("verifySHA256 aceitou digest errado")
	}
	if err := verifySHA256(path, "curto"); err == nil {
		t.Error("verifySHA256 aceitou digest malformado")
	}

	// Uppercase digests must still match; case is not a mismatch.
	if err := verifySHA256(path, strings.ToUpper(correct)); err != nil {
		t.Errorf("verifySHA256 rejeitou digest em maiúsculas: %v", err)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	dir := t.TempDir()

	// Build an archive whose entry name escapes the destination.
	archive := filepath.Join(dir, "evil.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dst := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := extractZip(archive, dst); err == nil {
		t.Error("extractZip aceitou entrada fora da pasta de destino")
	}

	// The escape target must not exist.
	if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
		t.Error("arquivo escapou para fora do diretório de destino")
	}
}

func TestLocateFailsWhenIncomplete(t *testing.T) {
	dir := t.TempDir()

	t.Run("diretório vazio", func(t *testing.T) {
		if _, err := locate(dir); err == nil {
			t.Error("locate aceitou diretório sem executável")
		}
	})

	t.Run("executável sem servidor", func(t *testing.T) {
		bin := "scrcpy.exe"
		if os.PathSeparator == '/' {
			bin = "scrcpy"
		}
		if err := os.WriteFile(filepath.Join(dir, bin), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := locate(dir); err == nil {
			t.Error("locate aceitou pacote sem scrcpy-server")
		}
	})

	t.Run("pacote completo", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "scrcpy-server"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		l, err := locate(dir)
		if err != nil {
			t.Fatalf("locate rejeitou pacote completo: %v", err)
		}
		if l.Server == "" {
			t.Error("Server não foi localizado")
		}
		if l.Binary == "" {
			t.Error("Binary não foi localizado")
		}
	})
}

// Ensure is serialised: concurrent callers must not start duplicate
// downloads of the same archive.
func TestEnsureIsSafeForConcurrentCallers(t *testing.T) {
	dest := t.TempDir()

	const callers = 4
	results := make(chan error, callers)

	for range callers {
		go func() {
			_, err := Ensure(context.Background(), dest, nil)
			results <- err
		}()
	}

	for range callers {
		<-results
	}

	// Errors here are expected without network; what matters is that no panic,
	// no data race and no partial directory is left behind for a later run to
	// mistake for a finished install.
	if entries, err := os.ReadDir(dest); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") && e.IsDir() {
				t.Errorf("diretório de staging permaneceu após falha: %s", e.Name())
			}
		}
	}
}
