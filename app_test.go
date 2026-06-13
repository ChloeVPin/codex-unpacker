package main

import (
	"runtime"
	"testing"
)

func TestVersionFromAssetName(t *testing.T) {
	tests := map[string]string{
		"OpenAI.Codex_26.609.4994.0_x64__2p2nqsd0c76g0.Msix": "26.609.4994.0",
		"Codex-26.609.41114-arm64.dmg":                       "26.609.41114",
		"Codex-darwin-x64-26.609.41114.zip":                  "26.609.41114",
	}

	for name, want := range tests {
		if got := versionFromAssetName(name); got != want {
			t.Fatalf("versionFromAssetName(%q) = %q, want %q", name, got, want)
		}
	}
}

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
