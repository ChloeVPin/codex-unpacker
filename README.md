# codex-unpacked

Private Windows-first release mirror for the Codex MSIX updater flow.

This repo stores the automation, not the giant package itself. The release
assets are the downloaded `.msix`, a SHA256 manifest, and a small
`release.json` file that records the package version, source URL, hash, and
timestamp.

## Layout

- `update-codex.bat` - thin launcher for Windows users
- `tools/update-codex.ps1` - PowerShell engine
- `data/latest.json` - local idempotence state

## Usage

Dry-run against the MSIX already in the workspace:

```powershell
.\update-codex.bat -DryRun -LocalMsixPath .\OpenAI.Codex_26.602.4764.0_x64__2p2nqsd0c76g0.Msix
```

Probe the live upstream feed without downloading the package:

```powershell
.\update-codex.bat -ProbeOnly
```

Run the full update and publish flow:

```powershell
.\update-codex.bat
```

The script prefers `gh` for release publishing and can fall back to a PAT
backed GitHub API flow when `GH_TOKEN` or `GITHUB_TOKEN` is present.

For package resolution it tries the Microsoft Store path first and falls back
to the public mirror release feed if the Store SOAP endpoint refuses the
request.

## Notes

- The repo stays lean; GitHub Releases carry the payload.
- The updater validates `AppxManifest.xml`, `AppxBlockMap.xml`, and
  `AppxSignature.p7x` as read-only inputs.
- `AppxManifest.xml` is treated as the source of truth for the package
  version.
