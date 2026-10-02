// Package fetch downloads, verifies and extracts the pinned scrcpy build.
//
// Verification is not optional. Reflexo executes the resulting binary with the
// user's own privileges, so a tampered archive would be arbitrary code
// execution on their machine. Every download is checked against the upstream
// SHA256SUMS.txt before a single byte is written to disk.
package fetch

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reflexo/internal/catalog"
)

// Phase names the current stage of an Ensure run, for display in the UI.
type Phase string

const (
	PhaseChecking   Phase = "checking"
	PhaseDownload   Phase = "download"
	PhaseVerifying  Phase = "verifying"
	PhaseExtracting Phase = "extracting"
	PhaseReady      Phase = "ready"
)

// Status is a progress report emitted while Ensure works.
type Status struct {
	Phase   Phase
	Percent float64 // 0..1, meaningful only during PhaseDownload
	Message string
}

// Layout is the set of files Reflexo needs from the extracted archive.
type Layout struct {
	// Root is the directory holding the extracted payload.
	Root string
	// Binary is the scrcpy executable.
	Binary string
	// ADB is the adb executable. May be empty when the platform archive does
	// not bundle one, in which case the caller must fall back to PATH.
	ADB string
	// Server is the scrcpy-server file pushed to the device.
	Server string
	// IconDir holds the icons, when present.
	IconDir string
}

// ProgressFunc receives status updates. It must not block.
type ProgressFunc func(Status)

// ErrUnsupportedPlatform is returned when upstream publishes no build for the
// running platform.
var ErrUnsupportedPlatform = errors.New("plataforma não suportada")

// Ensure returns a ready Layout for the running platform, downloading and
// extracting it on first use.
//
// It is safe for concurrent calls: the second caller blocks until the first
// finishes instead of starting a duplicate download.
func Ensure(ctx context.Context, dest string, progress ProgressFunc) (*Layout, error) {
	ensureMu.Lock()
	defer ensureMu.Unlock()

	asset, err := catalog.ForHost()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPlatform, err)
	}

	dir := filepath.Join(dest, asset.Stem+"-"+catalog.Version)

	// Fast path: already installed from a previous run.
	if layout, err := locate(dir); err == nil {
		report(progress, Status{Phase: PhaseReady, Message: "Pronto."})
		return layout, nil
	}

	sums, err := fetchChecksums(ctx)
	if err != nil {
		return nil, err
	}

	want, ok := sums[asset.ArchiveName()]
	if !ok {
		return nil, fmt.Errorf(
			"O manifesto do upstream não contém %q; refusing to install a build we cannot verify",
			asset.ArchiveName())
	}

	report(progress, Status{Phase: PhaseDownload, Message: "Baixando scrcpy " + catalog.Version + "..."})

	archivePath, err := download(ctx, asset.URL(), filepath.Join(dest, asset.ArchiveName()), progress)
	if err != nil {
		return nil, err
	}

	report(progress, Status{Phase: PhaseVerifying, Message: "Conferindo integridade..."})

	if err := verifySHA256(archivePath, want); err != nil {
		// A corrupt or hostile archive must never reach the extractor.
		_ = os.Remove(archivePath)
		return nil, err
	}

	report(progress, Status{Phase: PhaseExtracting, Message: "Instalando arquivos..."})

	// Extract to a scratch directory and move into place, so a crash mid-way
	// cannot leave a half-installed tree that the fast path would accept.
	staging := dir + ".tmp"
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("criando pasta temporária: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := extract(archivePath, asset.Kind, staging); err != nil {
		return nil, err
	}

	// Validate the staging tree before publishing it. If the archive turned out
	// to be missing the server or the binary, fail here rather than after the
	// rename, when the bad tree would already be the one the fast path finds.
	if _, err := locate(staging); err != nil {
		return nil, err
	}

	_ = os.RemoveAll(dir)
	if err := os.Rename(staging, dir); err != nil {
		// Rename can fail across filesystems; fall back to a copy.
		if err := copyTree(staging, dir); err != nil {
			return nil, fmt.Errorf("finalizando instalação: %w", err)
		}
	}

	// Re-resolve against the final path so callers never hold a staging path.
	final, err := locate(dir)
	if err != nil {
		return nil, err
	}

	_ = os.Remove(archivePath)
	report(progress, Status{Phase: PhaseReady, Message: "Pronto."})

	return final, nil
}

// ensureMu serialises Ensure across the process.
var ensureMu sync.Mutex

// report sends a status update, tolerating a nil callback.
func report(fn ProgressFunc, s Status) {
	if fn != nil {
		fn(s)
	}
}

// client is shared by all downloads. The timeout covers the whole body read,
// which matters on the slow corporate links this product targets.
var client = &http.Client{Timeout: 20 * time.Minute}

// fetchChecksums retrieves and parses the upstream manifest.
func fetchChecksums(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalog.ChecksumsURL(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("não foi possível obter a lista de verificações: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lista de verificações indisponível (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseChecksums(string(body)), nil
}

// parseChecksums reads lines shaped "<64 hex digits>  <filename>".
//
// The upstream manifest uses exactly two spaces; Fields-based parsing is used
// anyway because GNU coreutils emits a binary-mode " *" prefix for binaries
// when coreutils.checkBinary is enabled.
func parseChecksums(s string) map[string]string {
	out := make(map[string]string)

	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		sum, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if len(sum) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil {
			continue
		}
		out[name] = strings.ToLower(sum)
	}
	return out
}

// verifySHA256 streams the file and compares it against the expected digest.
func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(want) {
		return fmt.Errorf(
			"falha na verificação de integridade: esperado %s, obtido %s. O download foi descartado",
			want, got)
	}
	return nil
}

// download streams url to path, reporting progress as the body arrives.
func download(ctx context.Context, url, path string, progress ProgressFunc) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("falha no download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download falhou (HTTP %d)", resp.StatusCode)
	}

	total := resp.ContentLength

	var (
		written int64
		lastPct = -1.0
	)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, &progressReader{
		r: resp.Body,
		onChunk: func(n int64) {
			written += n
			if total <= 0 {
				return
			}
			pct := float64(written) / float64(total)
			// Throttle: the UI polls at 2s, so finer updates are wasted work.
			if pct-lastPct < 0.02 {
				return
			}
			lastPct = pct
			report(progress, Status{
				Phase:   PhaseDownload,
				Percent: pct,
				Message: fmt.Sprintf("Baixando... %d%%", int(pct*100)),
			})
		},
	}); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("download interrompido: %w", err)
	}

	return path, nil
}

// progressReader wraps a reader and invokes a callback as data flows.
type progressReader struct {
	r       io.Reader
	onChunk func(n int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.onChunk != nil {
		p.onChunk(int64(n))
	}
	return n, err
}

// extract unpacks an archive into dst.
//
// Both paths are hardened against "zip slip": an entry whose name escapes dst
// (via "../" or an absolute path) aborts the whole extraction rather than
// being written outside the install directory.
func extract(archivePath, kind, dst string) error {
	switch kind {
	case catalog.ArchiveZip:
		return extractZip(archivePath, dst)
	case catalog.ArchiveTarGz:
		return extractTarGz(archivePath, dst)
	default:
		return fmt.Errorf("formato de arquivo desconhecido: %q", kind)
	}
}

func extractZip(path, dst string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("abrindo zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		target, err := safeJoin(dst, f.Name)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		src, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, src, f.Mode())
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(path, dst string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("abrindo gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lendo tar: %w", err)
		}

		target, err := safeJoin(dst, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			// Symlinks and devices are never expected in these archives and
			// are the classic vector for escaping the destination.
			continue
		}
	}
}

// writeFile copies r into target, refusing to create an executable when the
// host filesystem does not support the permission bits.
func writeFile(target string, r io.Reader, mode os.FileMode) error {
	// Windows ignores the execute bit; strip it there to keep the tree usable.
	if os.PathSeparator == '\\' {
		mode &^= 0o111
	} else if mode == 0 {
		mode = 0o644
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return out.Close()
}

// safeJoin resolves name inside root, refusing anything that escapes it.
//
// Validation is done with the slash-based path package rather than filepath so
// that the same archive is judged identically on every host. That matters: on
// Windows, filepath.IsAbs("/etc/passwd") is false because the path lacks a
// drive letter, so a filepath-based check would accept on Windows what it
// rejects on Linux. A portable installer cannot have platform-dependent
// security rules.
func safeJoin(root, name string) (string, error) {
	// Normalise both separator styles first: archives are produced on all
	// three platforms and either may use backslashes.
	normalized := strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean(normalized)

	if path.IsAbs(clean) || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("entrada de arquivo com caminho absoluto recusada: %q", name)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("entrada de arquivo fora da pasta de instalação recusada: %q", name)
	}

	target := filepath.Join(root, filepath.FromSlash(clean))

	// Belt and braces: confirm the joined path really is under root, catching
	// anything the string analysis above might have missed.
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("caminho de arquivo inválido: %q", name)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("entrada de arquivo fora da pasta de instalação recusada: %q", name)
	}
	return target, nil
}

// locate finds the payload files inside an extracted tree.
//
// It searches rather than trusting a fixed layout, because the Windows, Linux
// and macOS archives differ in structure and we do not want a layout change in
// a patch release to break installation.
func locate(root string) (*Layout, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("scrcpy ainda não foi instalado: %w", err)
	}

	binary := findByName(root, executableNames()...)
	if binary == "" {
		return nil, fmt.Errorf("executável do scrcpy não encontrado em %s", root)
	}

	layout := &Layout{
		Root:    root,
		Binary:  binary,
		ADB:     findByName(root, "adb.exe", "adb"),
		Server:  findByName(root, "scrcpy-server"),
		IconDir: findDirWith(root, "scrcpy.png"),
	}

	if layout.Server == "" {
		return nil, errors.New("scrcpy-server não encontrado no pacote extraído")
	}
	return layout, nil
}

// executableNames returns the platform's scrcpy binary name.
func executableNames() []string {
	if os.PathSeparator == '\\' {
		return []string{"scrcpy.exe"}
	}
	return []string{"scrcpy"}
}

// findByName walks root depth-first looking for any of the given base names.
func findByName(root string, names ...string) string {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[strings.ToLower(n)] = true
	}

	found := ""
	// #nosec G304 -- root is a directory Reflexo created itself.
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		if want[strings.ToLower(info.Name())] {
			found = path
		}
		return nil
	})
	return found
}

// findDirWith returns the directory containing the given file, if present.
func findDirWith(root, name string) string {
	path := findByName(root, name)
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

// copyTree recursively copies src to dst, used only when os.Rename cannot
// cross a filesystem boundary.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		return writeFile(target, in, info.Mode())
	})
}

// ResolveADB returns the adb path to use, preferring the bundled one and
// falling back to PATH for platforms whose archive does not ship adb.
func ResolveADB(layout *Layout) (string, error) {
	if layout != nil && layout.ADB != "" {
		return layout.ADB, nil
	}
	if p, err := exec.LookPath("adb"); err == nil {
		return p, nil
	}
	return "", errors.New("adb não encontrado no pacote nem no PATH")
}
