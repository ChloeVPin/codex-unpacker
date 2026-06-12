# Codex Unpacker

Small Windows app and CLI for downloading the latest Codex MSIX locally.

It checks the current upstream package, saves it to a path you choose, and can inspect a Codex MSIX already on disk. It does not push anything to GitHub releases.

## Quick Start

The no-build path is the CLI through `npx`. If you want the desktop app, build it with Wails.

## What It Does

- Probes the current Codex Windows package metadata.
- Downloads the latest MSIX to a local path you choose.
- Verifies `AppxManifest.xml`, `AppxBlockMap.xml`, `AppxSignature.p7x`, and `AppxMetadata/CodeIntegrity.cat`.
- Computes SHA256 for the downloaded package.
- Inspects a local MSIX file without downloading anything.
- Records the last saved package in `data/latest.json`.
- Never publishes packages to GitHub.

## Requirements

For the GUI:

- Windows 10/11
- A downloaded `codex-unpacker.exe`

For source builds:

- Windows 10/11
- Go 1.23+
- Wails v2
- Node.js/npm
- Git

For `npx` usage:

- Node.js 18+
- Git

## Use The GUI

1. Build the GUI with `wails build`.
2. Launch `build\bin\codex-unpacker.exe`.
3. Click `Probe` to check the current upstream package.
4. Click `Download` and choose a save location.
5. Use `Choose` and `Dry run` to inspect a local `.msix`.

## Use The CLI With npx

```powershell
npx github:ChloeVPin/codex-unpacker status
npx github:ChloeVPin/codex-unpacker probe
npx github:ChloeVPin/codex-unpacker download --output .\Downloads\OpenAI.Codex_26.602.4764.0_x64__2p2nqsd0c76g0.Msix
npx github:ChloeVPin/codex-unpacker local .\OpenAI.Codex_26.602.4764.0_x64__2p2nqsd0c76g0.Msix --dry-run
```

## Build From Source

```powershell
wails dev
```

```powershell
wails build
```

The executable is written to:

```text
build\bin\codex-unpacker.exe
```

## Notes

- The tool is intentionally narrow: download, verify, and inspect.
- `data/latest.json` is just local state for the last saved package.
- Nothing in the app pushes packages to GitHub.
