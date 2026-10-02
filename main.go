// Command reflexo mirrors and controls an Android device on the computer.
//
// Usage:
//
//	reflexo                       open the guide and start when ready
//	reflexo --no-audio            pass extra arguments straight to scrcpy
//	reflexo --version             print version information and exit
//	reflexo --no-browser          print the local URL instead of opening a browser
//
// Reflexo never asks for administrator rights. It installs the scrcpy payload
// into the user's own profile directory and runs entirely from there.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"reflexo/internal/app"
	"reflexo/internal/catalog"
	"reflexo/internal/diag"
	"reflexo/internal/i18n"
	"reflexo/internal/logfile"
)

//go:embed all:web
var webFS embed.FS

// version is overridable at build time:
//
//	go build -ldflags "-X main.version=1.2.3"
var version = "0.1.0-dev"

func main() {
	var (
		showVersion = flag.Bool("version", false, "exibir a versão e sair")
		noBrowser   = flag.Bool("no-browser", false, "exibir o endereço local sem abrir o navegador")
		demo        = flag.Bool("demo", false, "mostrar uma demonstração, sem precisar de celular")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Printf("reflexo %s\n", version)
		fmt.Printf("scrcpy %s (empacotado)\n", catalog.Version)
		fmt.Printf("plataformas: %s\n", catalog.SupportedList())
		return
	}

	// Open the log before anything else can fail. A run that cannot even
	// create its log is the run most likely to need one.
	log, _ := openLog()

	if err := run(log, *noBrowser, *demo, flag.Args()); err != nil {
		detail := err.Error()
		if log != nil {
			log.Errorf("encerrando com erro: %v", err)
			// The log path belongs in the message, not only in the log. Someone
			// reading a dialog has not found the log yet, and "it is saved
			// somewhere" only helps if they know where.
			detail = fmt.Sprintf("%s\n\nDetalhe completo em:\n%s", detail, log.Path())
		}

		diag.Fatal("Reflexo não conseguiu iniciar", detail)
		_ = log.Close()
		os.Exit(1)
	}

	if log != nil {
		_ = log.Close()
	}
}

// openLog starts the run log, reporting to the console only if it fails.
//
// Failure is never fatal: Reflexo must keep working for someone whose profile
// happens to be read-only, so the worst case is no log at all.
func openLog() (*logfile.Logger, string) {
	dir, err := logfile.Dir()
	if err != nil {
		return nil, ""
	}

	// Where a failure gets written when there is no terminal to print it to.
	diag.NotePath = filepath.Join(dir, "erro.txt")

	log, err := logfile.Open(dir, "reflexo.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, "aviso: não consegui abrir o log ("+err.Error()+")")
		return nil, dir
	}
	return log, dir
}

func run(log *logfile.Logger, noBrowser, demo bool, extra []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The signal is logged by name rather than just "interrompido". A Reflexo
	// that quits on its own is the hardest kind of bug to diagnose: the console
	// carrying the trace is gone, and knowing whether it was Ctrl+C, a window
	// being closed, or a signal sent by another program is the whole question.
	// On Windows in particular, closing the console window delivers a control
	// event that Go turns into a termination, which is indistinguishable from
	// a crash unless it is written down.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)

	shutdown := make(chan string, 1)
	go func() {
		select {
		case sig := <-sigc:
			shutdown <- sig.String()
		case <-ctx.Done():
			shutdown <- "contexto encerrado"
		}
	}()

	dataDir, err := app.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("não foi possível criar a pasta de dados em %s: %w", dataDir, err)
	}

	// Carry over a payload left in the Roaming directory by an earlier version.
	// Purely a courtesy: a failure here costs a re-download of twenty-five
	// megabytes, and that is much cheaper than refusing to start.
	//
	// It is also the last step that will ever find anything there, which is why
	// it is logged loudly — an unexplained twenty-five megabytes left in a
	// cloud-synced folder is exactly the kind of thing a user notices later.
	if moved, merr := app.MigratePayload(dataDir, app.LegacyDataDir()); moved {
		log.Infof("payload migrado para %q (fora da pasta sincronizada)", dataDir)
	} else if merr != nil {
		log.Warnf("não foi possível migrar o payload: %v — será baixado de novo em %q",
			merr, dataDir)
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("interface embutida indisponível: %w", err)
	}

	// The banner answers, in the log, the question every support request opens
	// with: what version, on what machine, in which folder, talking to which
	// phone. All of it is here, at the top, before anything can go wrong.
	log.Infof("reflexo %s iniciando (pid %d, %s/%s)", version, os.Getpid(),
		runtime.GOOS, runtime.GOARCH)
	log.Infof("scrcpy %s | dados %q | log %q", catalog.Version, dataDir, log.Path())
	log.Infof("idioma %q | argumento extra %v", i18n.Detect(), extra)

	a := app.New(dataDir)
	a.SetLogger(log)
	a.ExtraArgs = extra

	// Started before the server so the very first page a visitor loads already
	// shows the demonstration. Waiting for the button would defeat the purpose:
	// the person who needs it is the person who has no phone to trigger it.
	if demo {
		a.StartDemo()
		log.Infof("demonstração: pedida na linha de comando")
	}

	srv := app.NewServer(a, sub)
	if err := srv.Start(ctx); err != nil {
		log.Errorf("não foi possível iniciar o servidor local: %v", err)
		return err
	}
	log.Infof("servidor em %s", srv.URL())

	// Open the browser straight away so the user sees the install progress,
	// which on first run takes several seconds.
	if !noBrowser {
		if err := app.OpenBrowser(srv.URL()); err != nil {
			log.Warnf("não consegui abrir o navegador automaticamente: %v", err)
			fmt.Fprintln(os.Stderr, "aviso: não consegui abrir o navegador automaticamente.")
			fmt.Fprintln(os.Stderr, "abra este endereço: "+srv.URL())
		}
	} else {
		fmt.Println(srv.URL())
	}

	// Installation runs in the background so the UI can render its progress.
	installCtx, stopInstall := context.WithCancel(ctx)
	defer stopInstall()
	go func() {
		// Every background goroutine gets a panic handler. Without this a panic
		// here kills the process and takes the log with it, which is exactly
		// when the log was about to be useful.
		defer log.Recover("instalação")

		if err := a.Install(installCtx, dataDir); err != nil {
			// The failure is already recorded in the state the UI renders;
			// the terminal line is only for someone watching the console.
			fmt.Fprintln(os.Stderr, "instalação: "+err.Error())
		}
	}()

	go func() {
		defer log.Recover("monitoramento de aparelhos")
		a.Watch(ctx)
	}()

	// Wait for the shutdown reason, not for ctx: ctx is only cancelled by the
	// deferred cancel below, so waiting on it would wait forever.
	reason := <-shutdown
	log.Infof("encerrando: %s", reason)
	a.Stop()
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `reflexo - espelha a tela do celular Android no computador

Uso:
  reflexo [opções] [argumentos do scrcpy]

Opções:
  --version       exibe a versão e sai
  --no-browser    exibe o endereço local sem abrir o navegador
  --demo          mostra uma demonstração, sem precisar de celular
  -h, --help      exibe esta ajuda

Qualquer argumento extra é repassado direto ao scrcpy, por exemplo:
  reflexo --no-audio --record=video.mkv

O Reflexo não solicita permissão de administrador.
`)
}
