package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reflexo/internal/device"
	"reflexo/internal/guide"
	"reflexo/internal/i18n"
	"reflexo/internal/logfile"
)

// A falha real de 2026-10-05, reproduzida:
//
//	10:46:15  espelhamento terminou com erro: exit status 2
//	10:46:20  ERROR  adb devices falhou: * daemon not running
//
// O celular estava conectado e autorizado o tempo todo. O servidor do adb
// morreu, o scrcpy perdeu o transporte, e o Reflexo passou a dizer "ADB
// indisponível" para sempre sem tentar consertar nada.
//
// Os testes abaixo verificam que a máquina de estados faz o que a lição exige:
// reage a falhas, espera antes de agir, backs off, e se recupera.

// newHealthApp returns an App past its startup grace window, because most tests
// here are about what happens once running, not during launch.
//
// New() stamps startedAt with time.Now(), so without this every test would sit
// inside the 15s window and never reach the recovery logic at all.
func newHealthApp(t *testing.T) *App {
	t.Helper()
	a, _ := newHealthAppWithLog(t)
	a.agePastStartup()
	return a
}

// agePastStartup moves the App past the startup grace window.
func (a *App) agePastStartup() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startedAt = time.Now().Add(-startupGrace - time.Second)
}

// newHealthAppWithLog also returns the log directory, for tests that need to
// read what was written.
func newHealthAppWithLog(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	a := New(t.TempDir())
	lg, err := logfile.Open(dir, "reflexo.log")
	if err != nil {
		t.Fatalf("logfile.Open(): %v", err)
	}
	t.Cleanup(func() { lg.Close() })
	a.SetLogger(lg)
	return a, dir
}

var errDeadDaemon = errors.New("adb devices -l: exit status 1: * daemon not running; starting now at tcp:5037")

// Uma falha isolada não pode virar reinício. Um único poll ruim é ruído: o
// celular sendo desconectado, uma transação USB repetindo, uma hesitação.
// Matar um servidor saudável por causa disso quebraria qualquer outra
// ferramenta da máquina que também use adb.
func TestSingleFailureDoesNotTriggerRecovery(t *testing.T) {
	a := newHealthApp(t)

	s := a.checkADB(context.Background(), nil, errDeadDaemon)

	if s.ADBRecovering {
		t.Error("uma falha isolada disparou reinício; o limiar deveria exigir duas")
	}
	if s.ADBAvailable {
		t.Error("ADBReportedAvailable = true com adb falhando")
	}

	a.mu.RLock()
	tried := a.recoveryTried
	a.mu.RUnlock()
	if tried != 0 {
		t.Errorf("recoveryTried = %d, esperado 0", tried)
	}
}

// Duas falhas seguidas são um padrão, e Reflexo deve agir.
func TestTwoConsecutiveFailuresTriggerRecovery(t *testing.T) {
	a := newHealthApp(t)
	// A client is required. Without one there is no server to restart, and
	// attemptRecovery declines rather than panicking — which is the behaviour
	// TestRecoveryWithoutADBClientDoesNotPanic covers.
	a.adb = device.NewClient(filepath.Join(t.TempDir(), "adb.exe"))

	a.checkADB(context.Background(), nil, errDeadDaemon)
	s := a.checkADB(context.Background(), nil, errDeadDaemon)

	a.mu.RLock()
	tried := a.recoveryTried
	a.mu.RUnlock()

	if tried == 0 {
		t.Error("recoveryTried = 0 após duas falhas; o servidor adb nunca seria reiniciado")
	}
	// The binary does not exist, so the restart itself fails. Reflexo must
	// report that rather than claim a recovery it could not perform.
	if s.ADBRecovering {
		t.Error("ADBRecovering = true mesmo com o restart tendo falhado")
	}
}

// O estado "reconectando" é o que impede o usuário de achar que precisa
// reinstalar driver. Durante a recuperação ele precisa ser mostrado.
func TestRecoveringStateIsShownNotTheDeadEnd(t *testing.T) {
	a := newHealthApp(t)
	a.checkADB(context.Background(), nil, errDeadDaemon)
	sit := a.checkADB(context.Background(), nil, errDeadDaemon)

	inst := guide.Of(sit, i18n.PortugueseBrazil)

	if !strings.Contains(strings.ToLower(inst.Headline), "reconectando") {
		t.Errorf("headline durante a recuperação = %q; o usuário precisa ver que "+
			"algo está sendo feito, não uma instrução para desistir", inst.Headline)
	}
	if len(inst.Steps) == 0 {
		t.Error("a instrução de recuperação não tem nenhum passo")
	}
}

// A mensagem antiga é o que matou a sessão de suporte. Ela só pode aparecer
// quando não há mais nada a fazer.
func TestUnavailableMessageIsNotShownWhileRecovering(t *testing.T) {
	deadEnd := guide.Of(guide.Situation{ADBAvailable: false}, i18n.PortugueseBrazil)
	recovering := guide.Of(guide.Situation{ADBRecovering: true}, i18n.PortugueseBrazil)

	if deadEnd.Headline == recovering.Headline {
		t.Error("a mensagem de recuperação é igual à de indisponibilidade; " +
			"o usuário não tem como saber que o Reflexo está agindo")
	}
	if strings.Contains(recovering.Detail, "driver") {
		t.Errorf("a mensagem de recuperação fala de driver: %q. Nenhum driver está "+
			"com problema — o processo do servidor do adb morreu", recovering.Detail)
	}
}

// O backoff é o que impede que o Reflexo spame start-server. Sem ele, um
// servidor quebrado leva trinta reinícios por minuto.
func TestRecoveryBacksOff(t *testing.T) {
	a := newHealthApp(t)
	a.adb = device.NewClient(filepath.Join(t.TempDir(), "adb.exe"))

	// Duas falhas para passar o limiar, e depois o servidor continua quebrado.
	a.checkADB(context.Background(), nil, errDeadDaemon)
	a.checkADB(context.Background(), nil, errDeadDaemon)

	a.mu.RLock()
	first := a.recoveryTried
	a.mu.RUnlock()
	if first == 0 {
		t.Fatal("a primeira recuperação não aconteceu")
	}

	// Polls imediatamente seguintes não podem gerar nova tentativa.
	for i := 0; i < 5; i++ {
		a.checkADB(context.Background(), nil, errDeadDaemon)
	}

	a.mu.RLock()
	second := a.recoveryTried
	a.mu.RUnlock()

	if second > first+1 {
		t.Errorf("recoveryTried subiu de %d para %d em cinco polls seguidos; "+
			"o backoff não está segurando", first, second)
	}
}

// O backoff cresce. Se o servidor está quebrado há um minuto, não adianta
// insistir mais rápido.
func TestBackoffDelayGrows(t *testing.T) {
	if len(recoveryDelays) < 3 {
		t.Fatalf("a tabela de backoff tem %d entradas; precisa crescer", len(recoveryDelays))
	}
	for i := 1; i < len(recoveryDelays); i++ {
		if recoveryDelays[i] <= recoveryDelays[i-1] {
			t.Errorf("atraso %d (%s) não é maior que o anterior (%s)",
				i, recoveryDelays[i], recoveryDelays[i-1])
		}
	}
	if recoveryDelays[0] < 2*time.Second {
		t.Errorf("o primeiro atraso é %s; o poll roda a cada 2s, então uma "+
			"tentativa antes disso é indistinguível de ruído", recoveryDelays[0])
	}
	if last := recoveryDelays[len(recoveryDelays)-1]; last > 90*time.Second {
		t.Errorf("o atraso máximo é %s; acima disso o usuário desiste antes de "+
			"uma nova tentativa", last)
	}
}

// Recuperar e continuar funcionando é o objetivo inteiro. O contador precisa
// zerar, senão o Reflexo fica em recovery permanente depois de uma troca.
func TestSuccessResetsFailureState(t *testing.T) {
	a := newHealthApp(t)

	a.checkADB(context.Background(), nil, errDeadDaemon)
	a.checkADB(context.Background(), nil, errDeadDaemon)

	a.checkADB(context.Background(), []device.Device{{Serial: "X", State: device.StateReady}}, nil)

	a.mu.RLock()
	failures := a.adbFailures
	tried := a.recoveryTried
	a.mu.RUnlock()

	if failures != 0 {
		t.Errorf("adbFailures = %d após um poll bem-sucedido", failures)
	}
	if tried != 0 {
		t.Errorf("recoveryTried = %d após um poll bem-sucedido; o estado não "+
			"volta ao normal", tried)
	}
}

// A lista vazia logo depois da queda do espelho é o caso que produz a
// confusão original: "adb devices" sai com código zero e imprime nada quando o
// servidor está morrendo, e essa saída é idêntica à de uma mesa sem celular.
func TestEmptyListRightAfterMirrorLossIsDistrusted(t *testing.T) {
	a := newHealthApp(t)
	a.noteMirrorLost()

	if !a.suspiciousEmpty() {
		t.Error("uma lista vazia logo após a queda do espelho não foi considerada suspeita")
	}
}

// Uma lista vazia numa mesa parada é normal e não deve acordar ninguém.
func TestEmptyListOnAnIdleDeskIsNormal(t *testing.T) {
	a := newHealthApp(t)

	if a.suspiciousEmpty() {
		t.Error("lista vazia sem queda recente foi considerada suspeita; " +
			"isso dispararia recuperação sem motivo")
	}
}

// Depois de um tempo, a lista vazia volta a ser só uma lista vazia.
func TestSuspicionExpires(t *testing.T) {
	a := newHealthApp(t)
	a.noteMirrorLost()

	a.mu.Lock()
	a.mirrorLostAt = time.Now().Add(-31 * time.Second)
	a.mu.Unlock()

	if a.suspiciousEmpty() {
		t.Error("uma lista vazia de 31s atrás ainda é suspeita; a janela de 30s " +
			"não está sendo respeitada")
	}
}

// A recuperação precisa sobreviver ao contexto do poll. Um poll que dá timeout
// no meio de um start-server deixaria a tentativa seguinte correndo em
// paralelo com essa.
func TestRecoveryIgnoresThePollContext(t *testing.T) {
	a := newHealthApp(t)
	a.checkADB(context.Background(), nil, errDeadDaemon)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // contexto já morto

	// Não deve entrar em pânico nem ficar preso; o objetivo é apenas provar
	// que um contexto cancelado não impede a tentativa.
	done := make(chan struct{})
	go func() {
		a.checkADB(ctx, nil, errDeadDaemon)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a recuperação ficou presa com o contexto do poll cancelado")
	}
}

// A janela de inicializacao importa mais do que parece.
//
// Medido em hardware real: o daemon do adb precisa de ~8s para escutar a 5037,
// e o primeiro poll roda 2s depois de subir. Sem a janela de graca, TODA
// inicializacao a frio abria mostrando "ADB indisponivel" por oito segundos,
// numa maquina saudavel. Pior que uma falha rara: o usuario aprende a
// ignorar a mensagem, e ai ela nao serve nem quando e verdade.
func TestStartupGraceHidesTheExpectedColdStartFailure(t *testing.T) {
	a, _ := newHealthAppWithLog(t)
	// Deliberately left inside the grace window, which is the point.
	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	s := a.checkADB(context.Background(), nil, errDeadDaemon)

	if !s.Starting {
		t.Error("uma falha de adb no inicio nao foi tratada como 'iniciando'; " +
			"a tela mostraria um erro onde so ha uma inicializacao em andamento")
	}
	if s.ADBRecovering {
		t.Error("ADBRecovering = true durante a janela de inicializacao; deveria ser Starting")
	}

	a.mu.RLock()
	tried := a.recoveryTried
	a.mu.RUnlock()
	if tried != 0 {
		t.Errorf("recoveryTried = %d dentro da janela de inicializacao; "+
			"o Reflexo tentou reiniciar o adb antes mesmo de ele terminar de subir", tried)
	}
}

// Passada a janela, a mesma falha volta a ser tratada como falha. Sem isso a
// graca viraria uma desculpa permanente.
func TestStartupGraceExpires(t *testing.T) {
	a := newHealthApp(t)
	a.adb = device.NewClient(filepath.Join(t.TempDir(), "adb.exe"))

	if a.withinStartupGrace() {
		t.Error("withinStartupGrace() = true depois da janela expirar")
	}

	a.checkADB(context.Background(), nil, errDeadDaemon)
	s := a.checkADB(context.Background(), nil, errDeadDaemon)

	a.mu.RLock()
	tried := a.recoveryTried
	a.mu.RUnlock()

	if tried == 0 {
		t.Error("recoveryTried = 0 passados 15s; a recuperacao nunca comecaria")
	}
	if s.ADBRecovering {
		t.Error("ADBRecovering = true mesmo com o restart tendo falhado")
	}
}

// A janela precisa ser longa o bastante para o daemon subir e curta o bastante
// para nao esconder uma falha real.
func TestStartupGraceIsSensiblySized(t *testing.T) {
	// Measured: polling "adb devices" from cold, with the daemon dead, took
	// 7.4s to answer on the test machine. A 15s window was tried and still
	// showed a recovery attempt at 17s, which a user reads as "it did not
	// work". The window has to clear the measurement with room to spare.
	if startupGrace < 20*time.Second {
		t.Errorf("a janela de inicializacao e %s; o start a frio medido leva ~7.4s "+
			"e 15s ja foi insuficiente na pratica", startupGrace)
	}
	if startupGrace > 60*time.Second {
		t.Errorf("a janela de inicializacao e %s; uma falha real ficaria escondida "+
			"por tempo demais", startupGrace)
	}
}

// O log precisa contar a história. Sem ele, "reiniciou e funcionou" e
// "reiniciou e não funcionou" são indistinguíveis depois de um dia.
// Recovery runs on the poll path. With no adb client it must degrade, not
// panic: a panic there would kill the process instead of showing a message,
// which is a far worse outcome than the one it was written to fix.
func TestRecoveryWithoutADBClientDoesNotPanic(t *testing.T) {
	a := newHealthApp(t) // New() leaves adb nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a recuperacao entrou em panico sem cliente adb: %v", r)
		}
	}()

	a.checkADB(context.Background(), nil, errDeadDaemon)
	s := a.checkADB(context.Background(), nil, errDeadDaemon)

	if s.ADBAvailable {
		t.Error("ADBAvailable = true sem cliente adb")
	}
}

// The recovery path must also be safe when a client exists but the binary
// behind it does not, which is what an interrupted install looks like.
func TestRecoveryWithAMissingBinaryDoesNotPanic(t *testing.T) {
	a := newHealthApp(t)
	a.adb = device.NewClient(filepath.Join(t.TempDir(), "adb-que-nao-existe.exe"))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a recuperacao entrou em panico com binario ausente: %v", r)
		}
	}()

	a.checkADB(context.Background(), nil, errors.New("fork/exec: file does not exist"))
	a.checkADB(context.Background(), nil, errors.New("fork/exec: file does not exist"))
}

func TestRecoveryIsLogged(t *testing.T) {
	a, dir := newHealthAppWithLog(t)
	a.agePastStartup()
	a.adb = device.NewClient(filepath.Join(t.TempDir(), "adb.exe"))

	a.checkADB(context.Background(), nil, errDeadDaemon)
	a.checkADB(context.Background(), nil, errDeadDaemon)

	body, err := os.ReadFile(filepath.Join(dir, "reflexo.log"))
	if err != nil {
		t.Fatalf("não foi possível ler o log: %v", err)
	}

	text := string(body)
	if !strings.Contains(text, "adb") {
		t.Errorf("o log não menciona adb:\n%s", text)
	}
	if !strings.Contains(text, "reiniciando") {
		t.Errorf("o log não registra que houve tentativa de reinício:\n%s", text)
	}
}

// A classificacao escolhe o que dizer ao usuario, e o que Reflexo faz.
//
// NeedsRestart e sempre falso, e isso e uma decisao, nao uma omissao: o
// servidor adb na porta 5037 e compartilhado com Android Studio, VS Code e
// qualquer outra ferramenta da maquina. Matar esse servidor para tratar o
// nosso sintoma quebraria programas que o usuario nao pediu para tocarmos.
func TestRecoveryNeverKillsTheSharedADBServer(t *testing.T) {
	for _, k := range []device.FailureKind{
		device.FailureDaemonDead,
		device.FailureDaemonHung,
		device.FailureForeignServer,
		device.FailureNotInstalled,
		device.FailureUnknown,
	} {
		if k.NeedsRestart() {
			t.Errorf("NeedsRestart() = true para %v; o servidor adb da porta 5037 e "+
				"compartilhado com o resto da maquina e nao pode ser derrubado", k)
		}
	}
}

func TestFailureClassificationNamesTheCause(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want device.FailureKind
	}{
		{
			name: "servidor encerrado",
			err:  errors.New("exit status 1: * daemon not running; starting now at tcp:5037"),
			want: device.FailureDaemonDead,
		},
		{
			name: "servidor travado",
			err:  context.DeadlineExceeded,
			want: device.FailureDaemonHung,
		},
		{
			name: "handshake falhou",
			err:  errors.New("failed to check server version: cannot connect to daemon"),
			want: device.FailureDaemonDead,
		},
		{
			name: "outra versao do servidor",
			err:  errors.New("protocol fault (couldn't read status): connection reset"),
			want: device.FailureForeignServer,
		},
		{
			name: "adb ausente",
			err:  errors.New("fork/exec adb.exe: file does not exist"),
			want: device.FailureNotInstalled,
		},
		{
			name: "erro desconhecido",
			err:  errors.New("algo estranho aconteceu"),
			want: device.FailureUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := device.Classify(tt.err, ""); got != tt.want {
				t.Errorf("Classify() = %v, esperado %v", got, tt.want)
			}
		})
	}
}

// Um adb que responde deve zerar o estado, mesmo que a lista venha vazia e sem
// queda recente: é o caso comum de um celular desligado no meio da noite.
func TestHealthyADBWithNoDevicesIsNotAProblem(t *testing.T) {
	a := newHealthApp(t)

	s := a.checkADB(context.Background(), nil, nil)

	if !s.ADBAvailable {
		t.Error("ADBAvailable = false com adb saudável e nenhum aparelho; " +
			"essa é a tela de espera normal")
	}
	if s.ADBRecovering {
		t.Error("ADBRecovering = true sem nenhuma falha")
	}
	if len(s.Devices) != 0 {
		t.Errorf("Devices = %v, esperado lista vazia", s.Devices)
	}
}
