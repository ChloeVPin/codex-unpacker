Name:           codex-unpacker
Version:        1.0.3
Release:        1%{?dist}
Summary:        Terminal UI for downloading and managing OpenAI Codex packages
License:        MIT
URL:            https://github.com/ChloeVPin/codex-unpacker
Group:          Applications/System
Source0:        codex-unpacker
Source1:        codex-unpacker.desktop
Source2:        codex-unpacker.png

%description
Codex Unpacker is a terminal-based application that helps you download,
verify, and manage OpenAI Codex packages for Windows (MSIX) and macOS (DMG).

Features include:
- Interactive TUI with real-time download progress
- SHA-256 checksum verification
- Package inspection and metadata display
- Version tracking and update detection
- Support for both Windows x64 and macOS arm64/x64 targets

%install
install -Dpm 0755 %{SOURCE0} %{buildroot}%{_bindir}/codex-unpacker
install -Dpm 0644 %{SOURCE1} %{buildroot}%{_datadir}/applications/codex-unpacker.desktop
install -Dpm 0644 %{SOURCE2} %{buildroot}%{_datadir}/icons/hicolor/512x512/apps/codex-unpacker.png

%files
%{_bindir}/codex-unpacker
%{_datadir}/applications/codex-unpacker.desktop
%{_datadir}/icons/hicolor/512x512/apps/codex-unpacker.png

%changelog
* Tue Jun 16 2026 ChloeVPin - 1.0.3-1
- Fix copyFile double-close, expand test coverage, add --version flag
