# Codex Unpacked

Minimal Windows GUI for mirroring the Codex MSIX into GitHub Releases.

The repo stores the Wails/Go automation, not the large package payload. Releases carry the `.msix`, `SHA256SUMS.txt`, and `release.json`.

## What It Does

- Checks the official Codex Windows update manifest.
- Resolves the current MSIX from the validated mirror release source.
- Validates the MSIX by reading `AppxManifest.xml` and requiring blockmap/signature metadata.
- Computes SHA256 without loading the package into memory.
- Publishes a GitHub Release with the MSIX, checksum file, and release manifest.
- Updates `data/latest.json` so reruns are idempotent.
- Supports local MSIX validation and manual publishing through the GUI.

## Requirements

- Windows 10/11
- Go 1.23+
- Wails v2
- Node.js/npm
- Git
- GitHub CLI authenticated with release permission:

```powershell
gh auth login
```

## Run From Source

```powershell
wails dev
```

## Build

```powershell
wails build
```

The executable is written to:

```text
build\bin\codex-unpacked.exe
```

## Release Flow

1. Open the app.
2. Confirm the repo and GitHub auth status show ready.
3. Click `Probe` to check the latest package metadata.
4. Click `Publish` to download, validate, hash, and create the GitHub Release.

For manual testing, use `Choose` to select a local `.msix`, then `Dry run` or `Publish`.

## Notes

- Large packages are ignored by git history.
- `AppxManifest.xml` is treated as the source of truth for package version.
- Blockmap/signature files are validation inputs only.
- The project should stay private until redistribution and signing implications are fully settled.
