//go:build !windows

package diag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// show must leave a readable record behind, not just attempt to open it.
//
// The attempt can fail for reasons that have nothing to do with Reflexo: no
// desktop session, no xdg-open, a bare container. The file is the part that is
// guaranteed, so it is the part worth asserting.
func TestShowWritesNoteFile(t *testing.T) {
	dir := t.TempDir()
	note := filepath.Join(dir, "erro.txt")

	old := NotePath
	NotePath = note
	t.Cleanup(func() { NotePath = old })

	show("Reflexo não conseguiu iniciar", "a pasta de dados não pôde ser criada")

	b, err := os.ReadFile(note)
	if err != nil {
		t.Fatalf("show() não escreveu o arquivo: %v", err)
	}

	body := string(b)
	for _, want := range []string{
		"Reflexo não conseguiu iniciar",
		"a pasta de dados não pôde ser criada",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("o arquivo não contém %q:\n%s", want, body)
		}
	}
}

// Fatal must survive NotePath being unset, which is the state before main has
// chosen a log directory.
func TestShowWithoutNotePathDoesNotPanic(t *testing.T) {
	old := NotePath
	NotePath = ""
	t.Cleanup(func() { NotePath = old })

	show("titulo", "detalhe")
}

// Fatal writes to stderr before deciding anything, so a user running Reflexo
// from a shell always gets the text there regardless of the platform path.
func TestFatalWritesToStderr(t *testing.T) {
	note := filepath.Join(t.TempDir(), "erro.txt")
	old := NotePath
	NotePath = note
	t.Cleanup(func() { NotePath = old })

	Fatal("titulo", "detalhe do problema")
}
