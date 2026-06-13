<p align="center">
  <img src="assets/logo.png" width="160" alt="codex-unpacker logo" />
</p>

<h1 align="center">codex-unpacker</h1>

<p align="center">Terminal-first Go app for finding the latest Codex package and saving it to your Downloads folder by default.</p>

It ships as a compact TUI for interactive use and a CLI for scripting. It supports the Windows MSIX path and the macOS DMG path, and it does not automate GitHub Releases.

## What It Does

- Checks the official Codex update manifest for Windows.
- Resolves the matching Codex MSIX on Windows and the matching Codex DMG on macOS.
- Downloads the package to your `Downloads` folder by default.
- Validates Windows MSIX structure and macOS SHA256 integrity.
- Computes SHA256 for the package.
- Inspects a local `.msix` or `.dmg` file without downloading anything.
- Records the last successful download per target in `data/latest.json`.

## Build

```powershell
go build -o codex-unpacker.exe .
```

## Run

```powershell
.\codex-unpacker.exe
```

That opens the TUI.

## CLI

```powershell
.\codex-unpacker.exe probe
.\codex-unpacker.exe probe --platform macos --arch arm64
.\codex-unpacker.exe download
.\codex-unpacker.exe download --platform macos --arch arm64
.\codex-unpacker.exe download --output .\Downloads
.\codex-unpacker.exe download --output .\Downloads\codex-unpacker-latest.msix
.\codex-unpacker.exe inspect .\OpenAI.Codex_26.609.4994.0_x64__2p2nqsd0c76g0.Msix
.\codex-unpacker.exe inspect .\Codex-26.609.41114-arm64.dmg
```

## Notes

- `probe` and `download` default to the current host target when you do not pass `--platform`.
- `download` saves to your Downloads folder if you do not pass an output path.
- `inspect` validates an existing Codex package and prints its version and hash.
- `data/latest.json` is local state only; it is not a release artifact, and it keeps Windows and macOS entries separate.
- The README header and TUI banner both use the bundled logo in `assets/logo.png`.
- This repository no longer contains Wails, Node, or GitHub release automation.
