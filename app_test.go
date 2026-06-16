package main

import (
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// versionFromAssetName
// ---------------------------------------------------------------------------

func TestVersionFromAssetName(t *testing.T) {
	tests := map[string]string{
		"OpenAI.Codex_26.609.4994.0_x64__2p2nqsd0c76g0.Msix": "26.609.4994.0",
		"Codex-26.609.41114-arm64.dmg":                       "26.609.41114",
		"Codex-darwin-x64-26.609.41114.zip":                  "26.609.41114",
		"unknown-file.zip":                                    "",
		"":                                                    "",
	}

	for name, want := range tests {
		if got := versionFromAssetName(name); got != want {
			t.Errorf("versionFromAssetName(%q) = %q, want %q", name, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// resolveTargetSpec
// ---------------------------------------------------------------------------

func TestResolveTargetSpecWindowsDefaultsToX64(t *testing.T) {
	target, err := resolveTargetSpec("windows", "")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Platform != "windows" {
		t.Fatalf("target.Platform = %q, want windows", target.Platform)
	}
	if target.Architecture != "x64" {
		t.Fatalf("target.Architecture = %q, want x64", target.Architecture)
	}
}

func TestResolveTargetSpecWindowsARM64Rejected(t *testing.T) {
	_, err := resolveTargetSpec("windows", "arm64")
	if err == nil {
		t.Fatal("expected error for windows/arm64, got nil")
	}
}

func TestResolveTargetSpecMacOSArm64(t *testing.T) {
	target, err := resolveTargetSpec("macos", "arm64")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Platform != "macos" {
		t.Fatalf("target.Platform = %q, want macos", target.Platform)
	}
	if target.Architecture != "arm64" {
		t.Fatalf("target.Architecture = %q, want arm64", target.Architecture)
	}
}

func TestResolveTargetSpecMacOSX64(t *testing.T) {
	target, err := resolveTargetSpec("macos", "x64")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Architecture != "x64" {
		t.Fatalf("target.Architecture = %q, want x64", target.Architecture)
	}
}

func TestResolveTargetSpecMacOSUnsupportedArch(t *testing.T) {
	_, err := resolveTargetSpec("macos", "mips")
	if err == nil {
		t.Fatal("expected error for macos/mips, got nil")
	}
}

func TestResolveTargetSpecLinuxMapsToMacOS(t *testing.T) {
	target, err := resolveTargetSpec("linux", "")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Platform != "macos" {
		t.Fatalf("target.Platform = %q, want macos", target.Platform)
	}
}

func TestResolveTargetSpecMacOSDefaultsOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cross-target default is only deterministic on Windows in this test environment")
	}

	target, err := resolveTargetSpec("macos", "")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Platform != "macos" {
		t.Fatalf("target.Platform = %q, want macos", target.Platform)
	}
	if target.Architecture != "arm64" {
		t.Fatalf("target.Architecture = %q, want arm64", target.Architecture)
	}
}

func TestResolveTargetSpecMacOSDefaultsOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this test covers the Linux cross-target path")
	}
	target, err := resolveTargetSpec("macos", "")
	if err != nil {
		t.Fatalf("resolveTargetSpec returned error: %v", err)
	}
	if target.Platform != "macos" {
		t.Fatalf("target.Platform = %q, want macos", target.Platform)
	}
	// On Linux the non-darwin fallback should be arm64.
	if target.Architecture != "arm64" {
		t.Fatalf("target.Architecture = %q, want arm64", target.Architecture)
	}
}

// ---------------------------------------------------------------------------
// normalizePlatform
// ---------------------------------------------------------------------------

func TestNormalizePlatform(t *testing.T) {
	tests := map[string]string{
		"windows": "windows",
		"Windows": "windows",
		"win":     "windows",
		"win32":   "windows",
		"macos":   "macos",
		"macOS":   "macos",
		"darwin":  "macos",
		"mac":     "macos",
		"linux":   "macos",
		"LINUX":   "macos",
		"":        "",
	}
	for input, want := range tests {
		if input == "auto" {
			continue // skip — depends on host
		}
		got := normalizePlatform(input)
		if got != want {
			t.Errorf("normalizePlatform(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// normalizeArchitecture
// ---------------------------------------------------------------------------

func TestNormalizeArchitecture(t *testing.T) {
	tests := map[string]string{
		"":        "",
		"auto":    "",
		"x64":     "x64",
		"X64":     "x64",
		"amd64":   "x64",
		"x86_64":  "x64",
		"arm64":   "arm64",
		"ARM64":   "arm64",
		"aarch64": "arm64",
	}
	for input, want := range tests {
		got := normalizeArchitecture(input)
		if got != want {
			t.Errorf("normalizeArchitecture(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// targetKey / targetLabel / packageKindLabel
// ---------------------------------------------------------------------------

func TestTargetKey(t *testing.T) {
	tests := []struct {
		target TargetSpec
		want   string
	}{
		{TargetSpec{Platform: "windows", Architecture: "x64"}, "windows/x64"},
		{TargetSpec{Platform: "macOS", Architecture: "ARM64"}, "macos/arm64"},
		{TargetSpec{Platform: "windows", Architecture: ""}, "windows/"},
	}
	for _, tc := range tests {
		got := targetKey(tc.target)
		if got != tc.want {
			t.Errorf("targetKey(%+v) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestTargetLabel(t *testing.T) {
	tests := []struct {
		target TargetSpec
		want   string
	}{
		{TargetSpec{Platform: "windows", Architecture: "x64"}, "Windows x64"},
		{TargetSpec{Platform: "macos", Architecture: "arm64"}, "macOS arm64"},
		{TargetSpec{Platform: "linux", Architecture: "x64"}, "Linux x64"},
		{TargetSpec{Platform: "macos", Architecture: ""}, "macOS"},
	}
	for _, tc := range tests {
		got := targetLabel(tc.target)
		if got != tc.want {
			t.Errorf("targetLabel(%+v) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestPackageKindLabel(t *testing.T) {
	tests := map[string]string{
		"msix": "MSIX",
		"MSIX": "MSIX",
		"dmg":  "DMG",
		"DMG":  "DMG",
		"zip":  "ZIP",
		"":     "",
	}
	for input, want := range tests {
		got := packageKindLabel(input)
		if got != want {
			t.Errorf("packageKindLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// stateForTarget
// ---------------------------------------------------------------------------

func TestStateForTarget(t *testing.T) {
	target := TargetSpec{Platform: "macos", Architecture: "arm64"}
	state := StoredState{
		Targets: map[string]StoredTargetState{
			targetKey(target): {
				Package: PackageDetails{Target: target, Version: "26.609.41114"},
			},
		},
	}

	got := stateForTarget(state, target)
	if got.Package.Version != "26.609.41114" {
		t.Fatalf("stateForTarget returned version %q, want %q", got.Package.Version, "26.609.41114")
	}
}

func TestStateForTargetMissing(t *testing.T) {
	state := StoredState{
		Targets: map[string]StoredTargetState{},
	}
	got := stateForTarget(state, TargetSpec{Platform: "windows", Architecture: "x64"})
	if got.Package.Version != "" {
		t.Fatalf("expected empty state, got version %q", got.Package.Version)
	}
}

func TestStateForTargetNilMap(t *testing.T) {
	state := StoredState{}
	got := stateForTarget(state, TargetSpec{Platform: "windows", Architecture: "x64"})
	if got.Package.Version != "" {
		t.Fatalf("expected empty state from nil map, got version %q", got.Package.Version)
	}
}

// ---------------------------------------------------------------------------
// sameHash
// ---------------------------------------------------------------------------

func TestSameHash(t *testing.T) {
	hash := "547618a744149221078a27febdfff65c924b46ff85ab2fe1595180e128be8d85"

	if !sameHash(hash, hash) {
		t.Error("identical hashes should match")
	}
	if !sameHash(hash, strings.ToUpper(hash)) {
		t.Error("case-insensitive match should succeed")
	}
	if sameHash("", hash) {
		t.Error("empty hash should not match")
	}
	if sameHash(hash, "") {
		t.Error("empty hash should not match")
	}
	if sameHash("aaa", "bbb") {
		t.Error("different hashes should not match")
	}
}

// ---------------------------------------------------------------------------
// parseDigest
// ---------------------------------------------------------------------------

func TestParseDigest(t *testing.T) {
	tests := map[string]string{
		"sha256:abcdef":                 "abcdef",
		"SHA256:ABCDEF":                 "abcdef",
		"abcdef":                        "abcdef",
		"  sha256:abcdef  ":             "abcdef",
		"":                              "",
	}
	for input, want := range tests {
		got := parseDigest(input)
		if got != want {
			t.Errorf("parseDigest(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseChecksum
// ---------------------------------------------------------------------------

func TestParseChecksum(t *testing.T) {
	body := strings.Join([]string{
		"2601c0a15797b9dba27bd0b1fe466c1829f91b3d8e3ec71854610b6ef732f7d9  OpenAI.Codex_26.609.9530.0_x64__2p2nqsd0c76g0.Msix",
		"547618a744149221078a27febdfff65c924b46ff85ab2fe1595180e128be8d85  OpenAI.Codex_26.609.4994.0_x64__2p2nqsd0c76g0.Msix",
		"bad line",
		"",
	}, "\n")

	want := "2601c0a15797b9dba27bd0b1fe466c1829f91b3d8e3ec71854610b6ef732f7d9"
	got := parseChecksum(body, "OpenAI.Codex_26.609.9530.0_x64__2p2nqsd0c76g0.Msix")
	if got != want {
		t.Errorf("parseChecksum exact match: got %q, want %q", got, want)
	}

	// Case-insensitive filename match.
	got2 := parseChecksum(body, "openai.codex_26.609.9530.0_x64__2p2nqsd0c76g0.msix")
	if got2 != want {
		t.Errorf("parseChecksum case-insensitive: got %q, want %q", got2, want)
	}

	// Missing file returns empty.
	if got3 := parseChecksum(body, "nonexistent.msix"); got3 != "" {
		t.Errorf("parseChecksum missing: got %q, want empty", got3)
	}

	// Empty body returns empty.
	if got4 := parseChecksum("", "anything.msix"); got4 != "" {
		t.Errorf("parseChecksum empty body: got %q, want empty", got4)
	}
}

// ---------------------------------------------------------------------------
// looksLikeVersion
// ---------------------------------------------------------------------------

func TestLooksLikeVersion(t *testing.T) {
	valid := []string{"26.609.4994.0", "1.0", "26.609.41114", "1"}
	// "v1.0" has a dot so looksLikeVersion returns true — that is intentional;
	// the function is not a strict semver validator.
	invalid := []string{"", "   ", "abc", "!"}

	for _, v := range valid {
		if !looksLikeVersion(v) {
			t.Errorf("looksLikeVersion(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if looksLikeVersion(v) {
			t.Errorf("looksLikeVersion(%q) = true, want false", v)
		}
	}
}

// ---------------------------------------------------------------------------
// packageMatchesSource / packageMatchesPackage
// ---------------------------------------------------------------------------

func TestPackageMatchesSource(t *testing.T) {
	pkg := PackageDetails{
		Version:     "26.609.4994.0",
		SHA256:      "abcdef1234",
		PackageKind: "msix",
	}

	// Exact match.
	src := ResolvedSource{Version: "26.609.4994.0", ExpectedSHA256: "abcdef1234", PackageKind: "msix"}
	if !packageMatchesSource(pkg, src) {
		t.Error("identical source should match package")
	}

	// Version mismatch.
	src2 := ResolvedSource{Version: "99.0.0", ExpectedSHA256: "abcdef1234", PackageKind: "msix"}
	if packageMatchesSource(pkg, src2) {
		t.Error("version mismatch should not match")
	}

	// Hash mismatch.
	src3 := ResolvedSource{Version: "26.609.4994.0", ExpectedSHA256: "zzzzz", PackageKind: "msix"}
	if packageMatchesSource(pkg, src3) {
		t.Error("hash mismatch should not match")
	}

	// Empty version never matches.
	src4 := ResolvedSource{Version: ""}
	if packageMatchesSource(pkg, src4) {
		t.Error("empty source version should not match")
	}
}

func TestPackageMatchesPackage(t *testing.T) {
	a := PackageDetails{Version: "1.0", SHA256: "abc", PackageKind: "msix"}
	b := PackageDetails{Version: "1.0", SHA256: "abc", PackageKind: "msix"}

	if !packageMatchesPackage(a, b) {
		t.Error("identical packages should match")
	}

	b.SHA256 = "xyz"
	if packageMatchesPackage(a, b) {
		t.Error("hash mismatch should not match")
	}
}

// ---------------------------------------------------------------------------
// formatBytes  (in tui.go)
// ---------------------------------------------------------------------------

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(552187367), "526.6 MB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.input)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// truncateMiddle  (in tui.go)
// ---------------------------------------------------------------------------

func TestTruncateMiddle(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 7, "hel...rld"},
		{"hello world", 3, "hel"},
		{"hello world", 1, "h"},
		{"hello world", 0, ""},
		{"", 10, ""},
		{"  spaces  ", 10, "spaces"},         // TrimSpace applied
		{"abcdefghij", 5, "ab...ij"},
	}
	for _, tc := range tests {
		got := truncateMiddle(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("truncateMiddle(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// clamp  (in tui.go)
// ---------------------------------------------------------------------------

func TestClamp(t *testing.T) {
	if clamp(10, 0, 100) != 10 {
		t.Error("clamp in-range failed")
	}
	if clamp(-5, 0, 100) != 0 {
		t.Error("clamp below-min failed")
	}
	if clamp(200, 0, 100) != 100 {
		t.Error("clamp above-max failed")
	}
	if clamp(52, 52, 96) != 52 {
		t.Error("clamp at min failed")
	}
}

// ---------------------------------------------------------------------------
// shortHash  (in tui.go)
// ---------------------------------------------------------------------------

func TestShortHash(t *testing.T) {
	long := "547618a744149221078a27febdfff65c924b46ff85ab2fe1595180e128be8d85"
	got := shortHash(long)
	if got != "547618a7" {
		t.Errorf("shortHash long = %q, want %q", got, "547618a7")
	}
	if shortHash("abcd") != "abcd" {
		t.Error("shortHash short string should be returned as-is")
	}
	if shortHash("") != "" {
		t.Error("shortHash empty should return empty")
	}
}

// ---------------------------------------------------------------------------
// parseArgs (CLI argument parsing)
// ---------------------------------------------------------------------------

func TestParseArgsEmpty(t *testing.T) {
	cfg, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("parseArgs(nil) error: %v", err)
	}
	if cfg.command != "tui" {
		t.Errorf("parseArgs(nil) command = %q, want tui", cfg.command)
	}
}

func TestParseArgsHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		cfg, err := parseArgs([]string{arg})
		if err != nil {
			t.Fatalf("parseArgs(%q) error: %v", arg, err)
		}
		if cfg.command != "help" {
			t.Errorf("parseArgs(%q) command = %q, want help", arg, cfg.command)
		}
	}
}

func TestParseArgsVersion(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		cfg, err := parseArgs([]string{arg})
		if err != nil {
			t.Fatalf("parseArgs(%q) error: %v", arg, err)
		}
		if cfg.command != "version" {
			t.Errorf("parseArgs(%q) command = %q, want version", arg, cfg.command)
		}
	}
}

func TestParseArgsProbe(t *testing.T) {
	cfg, err := parseArgs([]string{"probe", "--platform", "macos", "--arch", "arm64", "--json"})
	if err != nil {
		t.Fatalf("parseArgs probe error: %v", err)
	}
	if cfg.command != "probe" {
		t.Errorf("command = %q, want probe", cfg.command)
	}
	if cfg.platform != "macos" {
		t.Errorf("platform = %q, want macos", cfg.platform)
	}
	if cfg.arch != "arm64" {
		t.Errorf("arch = %q, want arm64", cfg.arch)
	}
	if !cfg.json {
		t.Error("json flag should be true")
	}
}

func TestParseArgsDownload(t *testing.T) {
	cfg, err := parseArgs([]string{"download", "--output", "/tmp/out", "--platform", "windows"})
	if err != nil {
		t.Fatalf("parseArgs download error: %v", err)
	}
	if cfg.command != "download" {
		t.Errorf("command = %q, want download", cfg.command)
	}
	if cfg.output != "/tmp/out" {
		t.Errorf("output = %q, want /tmp/out", cfg.output)
	}
}

func TestParseArgsInspectRequiresPath(t *testing.T) {
	_, err := parseArgs([]string{"inspect"})
	if err == nil {
		t.Error("inspect without path should return error")
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	_, err := parseArgs([]string{"--unknown"})
	if err == nil {
		t.Error("unknown flag should return error")
	}
}

func TestParseArgsProbeMissingPlatformValue(t *testing.T) {
	_, err := parseArgs([]string{"probe", "--platform"})
	if err == nil {
		t.Error("--platform without value should return error")
	}
}

func TestParseArgsDownloadMissingOutputValue(t *testing.T) {
	_, err := parseArgs([]string{"download", "--output"})
	if err == nil {
		t.Error("--output without value should return error")
	}
}
