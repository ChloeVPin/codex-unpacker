<p align="center">
  <img src="assets/readme/hero.svg" width="100%" alt="codex-unpacker - Terminal-first Go package resolver for Codex" />
</p>

<p align="center">
  <a href="https://go.dev">
    <img src="https://img.shields.io/badge/Go-1.22+-00ADD8" alt="Go version" />
  </a>
  <img src="https://img.shields.io/badge/TUI-Bubbletea-00D26A" alt="TUI Framework" />
  <img src="https://img.shields.io/badge/targets-Windows%20%7C%20macOS-FFB020" alt="Targets" />
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-00ADD8" alt="License" />
  </a>
</p>

---

## Overview

`codex-unpacker` is a terminal-first Go application designed to query official Codex update manifests, resolve matching installer packages for Windows and macOS, and verify package structure and SHA256 integrity.

It operates as an interactive TUI for manual use and a non-interactive CLI tool for build scripting and automation.

### Core Functionality
- **Manifest Probing**: Checks official Windows and macOS Codex update endpoints for the latest version string.
- **Package Resolution**: Locates matching `.msix` (Windows) or `.dmg` (macOS) installers.
- **Integrity Validation**: Verifies Windows MSIX package structure and macOS DMG SHA256 checksums.
- **Local Inspection**: Inspects existing `.msix` or `.dmg` files locally without downloading.
- **State Persistence**: Tracks download history per target platform in `data/latest.json`.

<br />

<p align="center">
  <img src="assets/readme/architecture.svg" width="100%" alt="codex-unpacker execution pipeline flow: Probe to Resolve to Verify to Inspect" />
</p>

---

## Build & Launch

### Prerequisites
- Go version 1.22 or newer
- Windows (x64) or macOS (arm64 / x64)

### Building from Source

```powershell
go build -o codex-unpacker.exe .
```

### Launching Interactive TUI

```powershell
.\codex-unpacker.exe
```

Executing without subcommands launches the full-screen Charm Bubbletea terminal user interface.

---

## CLI Reference Flags & Commands

```powershell
# Probe the latest available release manifest for the host OS
.\codex-unpacker.exe probe

# Probe release manifest for a specific platform and architecture
.\codex-unpacker.exe probe --platform macos --arch arm64

# Download the latest matching package to the default Downloads directory
.\codex-unpacker.exe download

# Download package for a specified target platform
.\codex-unpacker.exe download --platform macos --arch arm64

# Specify a custom download directory or custom output filename
.\codex-unpacker.exe download --output .\Downloads
.\codex-unpacker.exe download --output .\Downloads\codex-latest.msix

# Inspect local package file structure and print version and SHA256 hash
.\codex-unpacker.exe inspect .\OpenAI.Codex_26.609.4994.0_x64.Msix
.\codex-unpacker.exe inspect .\Codex-26.609.41114-arm64.dmg
```

---

## Command Reference Summary

| Command | Operational Purpose | Default Output Behavior |
|---|---|---|
| `probe` | Queries upstream update manifest and prints latest version metadata. | Host platform target |
| `download` | Resolves, downloads, and verifies the latest matching package file. | Local `Downloads` folder |
| `inspect` | Validates an existing local `.msix` or `.dmg` installer file. | Terminal summary report |

---

## Architecture & State Notes

- **Target Auto-Detection**: `probe` and `download` automatically resolve the host OS and architecture when `--platform` is omitted.
- **Default Storage**: `download` defaults to saving files inside your user `Downloads` directory unless overridden by `--output`.
- **State Isolation**: `data/latest.json` maintains isolated download state entries for Windows and macOS targets.
- **Zero GitHub Dependency**: Operates independently of GitHub Releases APIs and requires no external Node.js or Wails runtimes.

---

## License

Distributed under the [MIT License](LICENSE).
