Name:           codex-unpacker
Version:        1.0.3
Release:        1%{?dist}
Summary:        TUI for downloading and managing OpenAI Codex packages
License:        MIT
URL:            https://github.com/ChloeVPin/codex-unpacker
Source0:        codex-unpacker
Source1:        codex-unpacker.desktop

%description
Codex Unpacker provides a terminal UI for probing, downloading,
and inspecting OpenAI Codex packages (MSIX for Windows, DMG for macOS).
It verifies SHA-256 checksums and tracks installed versions.

%install
install -Dpm 0755 %{SOURCE0} %{buildroot}%{_bindir}/codex-unpacker
install -Dpm 0644 %{SOURCE1} %{buildroot}%{_datadir}/applications/codex-unpacker.desktop

%files
%{_bindir}/codex-unpacker
%{_datadir}/applications/codex-unpacker.desktop

%changelog
* Tue Jun 16 2026 ChloeVPin - 1.0.3-1
- Fix copyFile double-close, expand test coverage, add --version flag
