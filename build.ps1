<#
.SYNOPSIS
    Builds Reflexo for Windows, without a console window.

.DESCRIPTION
    Reflexo ships as a windowless application on purpose. A console window is a
    kill switch: closing it delivers CTRL_CLOSE_EVENT, which the Go runtime
    turns into a termination, and Reflexo then "quits by itself" while the user
    is looking at a browser page that has quietly stopped updating. Removing
    the window removes that entire failure mode.

    The same binary still prints normally when run from a shell, because a
    windowless executable attaches to whatever console it finds. --version and
    --no-browser therefore keep working for developers and for locked-down
    machines where the browser cannot be opened at all.

.PARAMETER Version
    Version stamped into the binary. Defaults to the tag if HEAD is tagged.

.PARAMETER Install
    Also copies the binary into the user's Programs folder and refreshes the
    desktop shortcut.

.EXAMPLE
    .\build.ps1 -Version 0.1.1 -Install
#>
param(
    [string]$Version = "",
    [switch]$Install
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

# The Go toolchain is not on PATH by default on a fresh Windows install, and
# asking the user to fix that is exactly the kind of friction Reflexo exists to
# remove.
$goBin = Join-Path $env:LOCALAPPDATA "Programs\Go\bin"
if (Test-Path $goBin) { $env:PATH = "$goBin;$env:PATH" }

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go nao encontrado. Instale em https://go.dev/dl/ e rode de novo."
}

if (-not $Version) {
    $Version = "0.1.0-dev"
    try {
        $tag = (& git describe --tags --exact-match 2>$null)
        if ($LASTEXITCODE -eq 0 -and $tag) { $Version = $tag }
    } catch {
        # No git, or not a tagged commit: the development default stands.
    }
}

# -H windowsgui is what removes the console. -s -w drop the symbol and DWARF
# tables, which is a third off the download for anyone on a slow connection.
$ldflags = "-s -w -H windowsgui -X main.version=$Version"

$outDir = Join-Path $PSScriptRoot "dist"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$outFile = Join-Path $outDir "reflexo.exe"

Write-Host "Reflexo $Version"
Write-Host "  go build -ldflags `"$ldflags`""

& go build -trimpath -ldflags $ldflags -o $outFile .
if ($LASTEXITCODE -ne 0) { throw "go build falhou." }

$size = [math]::Round((Get-Item $outFile).Length / 1MB, 2)
Write-Host "  -> $outFile ($size MB)"

if ($Install) {
    $dest = Join-Path $env:LOCALAPPDATA "Programs\Reflexo"
    New-Item -ItemType Directory -Force -Path $dest | Out-Null

    # An installed copy that is still running cannot be overwritten, and
    # Windows reports that as an unexplained sharing failure. Say what happened.
    $running = Get-Process reflexo -ErrorAction SilentlyContinue
    if ($running) {
        throw "O Reflexo esta rodando (PID $($running.Id -join ', ')). Feche a janela do espelhamento e a interface, e rode de novo."
    }

    Copy-Item $outFile (Join-Path $dest "reflexo.exe") -Force
    Write-Host "  instalado em $dest"

    $lnk = Join-Path $env:USERPROFILE "Desktop\Reflexo.lnk"
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($lnk)
    $shortcut.TargetPath = Join-Path $dest "reflexo.exe"
    $shortcut.WorkingDirectory = $dest
    $shortcut.IconLocation = Join-Path $dest "reflexo.exe,0"
    $shortcut.Description = "Espelha a tela do celular Android no computador"
    $shortcut.Save()
    Write-Host "  atalho em $lnk"
}