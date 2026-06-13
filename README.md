# codex-unpacker

`codex-unpacker` is a small Go tool for finding the latest Codex package and saving it to your Downloads folder by default.

It ships as a terminal app with a TUI for interactive use and a compact CLI for scripting. It understands the Windows MSIX path and the macOS DMG path, but it does not publish anything to GitHub Releases.

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
- This repository no longer contains Wails, Node, or GitHub release automation.

