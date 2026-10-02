package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// O payload são vinte e cinco megabytes numa pasta que, no Windows, pode estar
// sincronizada com o OneDrive ou com um perfil de domínio. A sincronização
// deixaria o primeiro uso lento, comeria a cota do usuário e, em rede
// limitada, transformaria o lançamento em minutos de espera.
//
// Este é o mesmo argumento que colocou o log no LOCALAPPDATA, e o teste do log
// é o modelo.
func TestDataDirIsNeverInTheSyncedFolder(t *testing.T) {
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): %v", err)
	}

	// Os.UserConfigDir() é %AppData% no Windows, que é exatamente onde o
	// Reflexo não pode escrever. Vale para qualquer plataforma: em nenhum caso
	// o payload deve ficar onde a configuração do usuário é replicada.
	if synced, err := os.UserConfigDir(); err == nil && runtime.GOOS == "windows" {
		if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(synced)) {
			t.Errorf("DataDir() = %q, que está dentro de %q.\n"+
				"%q é a pasta Roaming, replicada pelo OneDrive e por perfil de domínio "+
				"em máquina corporativa — o público-alvo deste projeto. O payload do "+
				"scrcpy são 25 MB e não pode ficar ali. Ver paths.go.",
				dir, synced, synced)
		}
	}

	if !strings.Contains(strings.ToLower(dir), payloadDirName) {
		t.Errorf("DataDir() = %q não contém %q", dir, payloadDirName)
	}
}

// Nada de administrador significa nada de pasta de sistema. Estes caminhos são
// onde um usuário sem elevação não consegue gravar.
func TestDataDirAvoidsSystemLocations(t *testing.T) {
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): %v", err)
	}

	lower := strings.ToLower(dir)
	forbidden := []string{
		`\program files`,
		`\program files (x86)`,
		`\programdata\`,
		"/usr/local/",
		"/opt/",
		"/etc/",
		"/var/",
	}

	for _, bad := range forbidden {
		if strings.Contains(lower, bad) {
			t.Errorf("DataDir() = %q contém %q, que exige privilégio de administrador",
				dir, bad)
		}
	}
}

// No Linux, o lugar de dados de aplicação é $XDG_DATA_HOME, não ~/.config.
// ~/.config é onde o usuário edita configurações; um payload baixado não é
// configuração.
func TestLinuxUsesXDGDataHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("comportamento específico do Linux")
	}

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): %v", err)
	}

	want := filepath.Join(os.Getenv("XDG_DATA_HOME"), payloadDirName)
	if dir != want {
		t.Errorf("DataDir() = %q, esperado %q (XDG_DATA_HOME)", dir, want)
	}
	if strings.Contains(dir, ".config") {
		t.Errorf("DataDir() = %q está em ~/.config, que é para configuração editável", dir)
	}
}

// Sem XDG_DATA_HOME, a especificação manda usar ~/.local/share.
func TestLinuxFallsBackToLocalShare(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("comportamento específico do Linux")
	}

	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): %v", err)
	}

	want := filepath.Join(home, ".local", "share", payloadDirName)
	if dir != want {
		t.Errorf("DataDir() = %q, esperado %q", dir, want)
	}
}

// ~/Library/Caches pode ser esvaziado pelo sistema sem aviso, o que custaria
// vinte e cinco megabytes de novo download sem motivo. Application Support é o
// lugar convencional no macOS e não é replicado por padrão.
func TestMacOSPrefersApplicationSupportOverCaches(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("comportamento específico do macOS")
	}

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): %v", err)
	}

	if strings.Contains(dir, "Library/Caches") {
		t.Errorf("DataDir() = %q está em Library/Caches, que o macOS pode esvaziar "+
			"sozinho e forçar um novo download de 25 MB", dir)
	}
	if !strings.Contains(dir, "Library/Application Support") {
		t.Errorf("DataDir() = %q não está em ~/Library/Application Support", dir)
	}
}

// A migração é cortesia, nunca requisito. Se ela falhar, o Reflexo baixa o
// payload de novo. Este teste prova que uma migração que não encontra nada não
// trava ninguém.
//
// O diretório legado é passado como parâmetro de propósito: depender do
// estado real da máquina faria o teste falhar justamente na máquina que tem uma
// instalação antiga — ou seja, na única máquina onde a migração importa.
func TestMigratePayloadIsHarmlessWhenThereIsNothingToMove(t *testing.T) {
	dest := t.TempDir()
	legacy := t.TempDir() // existe, mas está vazio

	moved, err := MigratePayload(dest, legacy)
	if err != nil {
		t.Fatalf("MigratePayload() erro inesperado: %v", err)
	}
	if moved {
		t.Error("MigratePayload() = true com o diretório legado vazio")
	}
}

// Um diretório legado que não existe é o caso normal numa instalação nova.
func TestMigratePayloadToleratesAMissingLegacyDirectory(t *testing.T) {
	moved, err := MigratePayload(t.TempDir(), filepath.Join(t.TempDir(), "nao-existe"))
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if moved {
		t.Error("MigratePayload() = true sem diretório legado")
	}
}

// legacy == dest faria um rename de si mesmo. Tem de ser tratado como "nada a
// fazer", não como um erro.
func TestMigratePayloadRefusesToMoveItselfOntoItself(t *testing.T) {
	dir := t.TempDir()

	moved, err := MigratePayload(dir, dir)
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if moved {
		t.Error("MigratePayload() = true com legacy == dest")
	}
}

// O caso que importa: um payload real no lugar antigo precisa aparecer no novo.
func TestMigratePayloadMovesThePayload(t *testing.T) {
	dest := t.TempDir()
	legacy := t.TempDir()

	// O payload como o fetch.Ensure o deixa: uma árvore com o binário dentro.
	payload := filepath.Join(legacy, "scrcpy-win64-4.1", "scrcpy-win64-v4.1")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "scrcpy.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	moved, err := MigratePayload(dest, legacy)
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if !moved {
		t.Fatal("MigratePayload() = false com um payload presente no diretório legado")
	}

	// Tem de estar no novo lugar...
	got := filepath.Join(dest, "scrcpy-win64-4.1", "scrcpy-win64-v4.1", "scrcpy.exe")
	if _, err := os.Stat(got); err != nil {
		t.Errorf("payload não chegou em %s: %v", got, err)
	}
	// ...e ter saído do antigo. Um rename deixa a origem vazia.
	if _, err := os.Stat(filepath.Join(legacy, "scrcpy-win64-4.1")); !os.IsNotExist(err) {
		t.Errorf("o diretório legado ainda contém o payload (err=%v)", err)
	}
}

// O que não é payload fica onde está. O Reflexo não decide o que fazer com
// arquivos do usuário.
func TestMigratePayloadLeavesNonPayloadDirectoriesAlone(t *testing.T) {
	dest := t.TempDir()
	legacy := t.TempDir()

	keep := filepath.Join(legacy, "minha-pasta")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "nota.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	moved, err := MigratePayload(dest, legacy)
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if moved {
		t.Error("MigratePayload() = true movendo uma pasta que não é payload")
	}
	if _, err := os.Stat(filepath.Join(keep, "nota.txt")); err != nil {
		t.Errorf("uma pasta que não é payload foi movida: %v", err)
	}
}

// "logs" é interno do Reflexo, não payload. Tratá-lo como payload faria a
// migração achar que já há tudo instalado.
func TestMigratePayloadIgnoresTheLogsDirectory(t *testing.T) {
	dest := t.TempDir()
	legacy := t.TempDir()

	if err := os.MkdirAll(filepath.Join(legacy, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}

	moved, err := MigratePayload(dest, legacy)
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if moved {
		t.Error(`MigratePayload() = true por causa de "logs"`)
	}
}

// PayloadIsPresent precisa distinguir o payload dos diretórios internos do
// Reflexo. Se tratasse "logs" como payload, a migração acharia que já há tudo.
func TestPayloadIsPresentIgnoresInternalDirectories(t *testing.T) {
	dir := t.TempDir()

	if PayloadIsPresent(dir) {
		t.Errorf("PayloadIsPresent(%q) = true numa pasta vazia", dir)
	}

	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if PayloadIsPresent(dir) {
		t.Error(`PayloadIsPresent() = true só por causa de "logs", que não é payload`)
	}

	if err := os.MkdirAll(filepath.Join(dir, "scrcpy-win64-4.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !PayloadIsPresent(dir) {
		t.Error("PayloadIsPresent() = false com o payload presente")
	}
}

// Se um destino já estiver preenchido, a migração não pode sobrescrever. Uma
// migração anterior, ou um download que terminou enquanto ela rodava, vence.
func TestMigratePayloadNeverOverwritesAnExistingPayload(t *testing.T) {
	dest := t.TempDir()
	legacy := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dest, "scrcpy-win64-4.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "scrcpy-win64-4.1"), 0o755); err != nil {
		t.Fatal(err)
	}

	moved, err := MigratePayload(dest, legacy)
	if err != nil {
		t.Fatalf("MigratePayload(): %v", err)
	}
	if moved {
		t.Error("MigratePayload() = true com o destino já ocupado")
	}
}
