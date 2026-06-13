# codex-unpacker

`codex-unpacker` is a small Windows-first Go tool for finding the latest Codex MSIX and saving it to your Downloads folder by default.

It ships as a terminal app with a TUI for interactive use and a compact CLI for scripting. It does not publish anything to GitHub Releases.

## What It Does

- Checks the official Codex update manifest.
- Resolves the matching Codex MSIX from the mirror release when needed.
- Downloads the package to `%USERPROFILE%\Downloads` by default.
- Validates the downloaded MSIX contents.
- Computes SHA256 for the package.
- Inspects a local `.msix` file without downloading anything.
- Records the last successful download in `data/latest.json`.

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
.\codex-unpacker.exe download
.\codex-unpacker.exe download --output .\Downloads
.\codex-unpacker.exe download --output .\Downloads\codex-unpacker-latest.msix
.\codex-unpacker.exe inspect .\OpenAI.Codex_26.609.4994.0_x64__2p2nqsd0c76g0.Msix
```

## Notes

- `download` saves to the Windows Downloads folder if you do not pass an output path.
- `inspect` validates an existing Codex MSIX and prints its version and hash.
- `data/latest.json` is local state only; it is not a release artifact.
- This repository no longer contains Wails, Node, or GitHub release automation.

