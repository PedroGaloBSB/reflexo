//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The PE subsystem values from winnt.h.
const (
	subsystemConsole = 3
	subsystemGUI     = 2
)

// peSubsystem reads the subsystem field out of a PE image.
//
// Offset 68 inside the optional header holds the subsystem for both PE32 and
// PE32+, so one constant covers 32 and 64 bit builds. Getting this wrong would
// make the test pass or fail for the wrong reason, which is why the negative
// control below builds the same program without the flag and insists on seeing
// the other value.
func peSubsystem(t *testing.T, path string) uint16 {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", path, err)
	}
	if len(b) < 0x40 {
		t.Fatal("imagem pequena demais para ser um executável PE")
	}

	// e_lfanew, at 0x3C, is where the PE header starts.
	pe := int(binary.LittleEndian.Uint32(b[0x3C:]))
	if pe <= 0 || pe+24+68+2 > len(b) {
		t.Fatalf("cabeçalho PE fora da imagem (e_lfanew=%d)", pe)
	}

	if string(b[pe:pe+4]) != "PE\x00\x00" {
		t.Fatalf("assinatura PE ausente em %s", path)
	}

	return binary.LittleEndian.Uint16(b[pe+24+68:])
}

func buildReflexo(t *testing.T, ldflags string) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "reflexo.exe")

	args := []string{"build"}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "-o", out, ".")

	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %v falhou: %v\n%s", args, err, b)
	}
	return out
}

// TestReleaseBinaryHasNoConsoleWindow is the guard on the decision recorded in
// build.ps1: the shipped Windows binary must be a GUI-subsystem application.
//
// The cost of getting this wrong is not a cosmetic one. A console app launched
// from the desktop owns a console window, and closing that window delivers
// CTRL_CLOSE_EVENT — which the Go runtime reports to the program as SIGTERM.
// Reflexo then exits while its browser page is still open, and the user sees a
// page that has silently stopped working. That is the "it quits by itself"
// report, and the log confirms it: the run ends with "encerrando: terminated".
func TestReleaseBinaryHasNoConsoleWindow(t *testing.T) {
	gui := buildReflexo(t, "-s -w -H windowsgui -X main.version=test")

	if got := peSubsystem(t, gui); got != subsystemGUI {
		t.Errorf("subsistema = %d, esperado %d (GUI). O binário teria uma janela "+
			"de console, e fechá-la encerra o Reflexo enquanto a página no "+
			"navegador continua aberta.", got, subsystemGUI)
	}
}

// TestConsoleSubsystemIsDetectable is the control for the test above.
//
// Without it, a parser that always reported 2 — a wrong offset, a truncated
// read, a stale build — would make the real test pass forever. Here the very
// same parser must report 3 for a build made without the flag.
func TestConsoleSubsystemIsDetectable(t *testing.T) {
	console := buildReflexo(t, "-s -w -X main.version=test")

	if got := peSubsystem(t, console); got != subsystemConsole {
		t.Errorf("subsistema = %d, esperado %d (console). O teste acima não é "+
			"confiável: o parser não distingue os dois builds, então passaria "+
			"mesmo com o binário errado.", got, subsystemConsole)
	}
}
