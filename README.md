# Codex Unpacker

Minimal Windows GUI and `npx` CLI for mirroring the latest Codex MSIX into GitHub Releases.

The repo stores the Wails/Go automation, not the large package payload. Releases carry the `.msix`, `SHA256SUMS.txt`, and `release.json`.

## Quick Start

Most users should download the prebuilt Windows executable from the latest GitHub Release:

```text
codex-unpacker.exe
```

Developers can still build from source, and automation users can run the CLI through `npx`.

## What It Does

- Checks the official Codex Windows update manifest.
- Resolves the current MSIX from the validated mirror release source.
- Validates the MSIX by reading `AppxManifest.xml` and requiring blockmap/signature metadata.
- Computes SHA256 without loading the package into memory.
- Publishes a GitHub Release with the MSIX, checksum file, and release manifest.
- Updates `data/latest.json` so reruns are idempotent.
- Supports local MSIX validation and manual publishing through the GUI.
- Provides an `npx` CLI for status, probe, publish, and local MSIX dry runs.

## Requirements

For the prebuilt app:

- Windows 10/11
- Git
- GitHub CLI authenticated with release permission:

```powershell
gh auth login
```

For source builds:

- Windows 10/11
- Go 1.23+
- Wails v2
- Node.js/npm
- Git
- GitHub CLI

For `npx` usage:

- Node.js 18+
- Git
- GitHub CLI

## Use The GUI

1. Download `codex-unpacker.exe` from the latest release.
2. Run it from a clone of this repo so it can read and update `data/latest.json`.
3. Confirm the repo and GitHub auth status show ready.
4. Click `Probe` to check the latest package metadata.
5. Click `Publish` to download, validate, hash, and create the GitHub Release.

For manual testing, use `Choose` to select a local `.msix`, then `Dry run` or `Publish`.

## Use The CLI With npx

Run directly from GitHub:

```powershell
npx github:ChloeVPin/codex-unpacker status
npx github:ChloeVPin/codex-unpacker probe
npx github:ChloeVPin/codex-unpacker publish
npx github:ChloeVPin/codex-unpacker local .\OpenAI.Codex_26.602.4764.0_x64__2p2nqsd0c76g0.Msix --dry-run
npx github:ChloeVPin/codex-unpacker local .\OpenAI.Codex_26.602.4764.0_x64__2p2nqsd0c76g0.Msix --publish
```

The package is also ready to publish to npm later, which would allow:

```powershell
npx codex-unpacker probe
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

- Large packages are ignored by git history.
- `AppxManifest.xml` is treated as the source of truth for package version.
- Blockmap/signature files are validation inputs only.
- The project should stay private until redistribution and signing implications are fully settled.
