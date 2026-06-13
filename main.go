package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type cliConfig struct {
	command  string
	output   string
	json     bool
	path     string
	platform string
	arch     string
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		exitErr(err)
	}

	switch cfg.command {
	case "help":
		printUsage(os.Stdout)
	case "probe":
		runProbe(cfg)
	case "download":
		runDownload(cfg)
	case "inspect":
		runInspect(cfg.path, cfg.json)
	default:
		runTUI()
	}
}

func parseArgs(args []string) (cliConfig, error) {
	if len(args) == 0 {
		return cliConfig{command: "tui"}, nil
	}

	switch strings.ToLower(args[0]) {
	case "help", "-h", "--help":
		return cliConfig{command: "help"}, nil
	case "probe":
		return parseProbeCommand(args[1:])
	case "download":
		return parseDownloadCommand(args[1:])
	case "inspect":
		return parseInspectCommand(args[1:])
	case "tui":
		return cliConfig{command: "tui"}, nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return cliConfig{}, fmt.Errorf("unknown flag %q", args[0])
		}
		return cliConfig{command: "tui"}, nil
	}
}

func parseProbeCommand(args []string) (cliConfig, error) {
	cfg := cliConfig{command: "probe"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			cfg.json = true
		case "--platform":
			i++
			if i >= len(args) {
				return cliConfig{}, errors.New("probe requires a value after --platform")
			}
			cfg.platform = args[i]
		case "--arch":
			i++
			if i >= len(args) {
				return cliConfig{}, errors.New("probe requires a value after --arch")
			}
			cfg.arch = args[i]
		case "-h", "--help":
			return cliConfig{command: "help"}, nil
		default:
			if strings.HasPrefix(arg, "-") {
				return cliConfig{}, fmt.Errorf("unknown flag %q", arg)
			}
			return cliConfig{}, fmt.Errorf("probe does not accept positional arguments")
		}
	}
	return cfg, nil
}

func parseDownloadCommand(args []string) (cliConfig, error) {
	cfg := cliConfig{command: "download"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			cfg.json = true
		case "--platform":
			i++
			if i >= len(args) {
				return cliConfig{}, errors.New("download requires a value after --platform")
			}
			cfg.platform = args[i]
		case "--arch":
			i++
			if i >= len(args) {
				return cliConfig{}, errors.New("download requires a value after --arch")
			}
			cfg.arch = args[i]
		case "--output", "-o":
			i++
			if i >= len(args) {
				return cliConfig{}, errors.New("download requires a value after --output")
			}
			cfg.output = args[i]
		case "-h", "--help":
			return cliConfig{command: "help"}, nil
		default:
			if strings.HasPrefix(arg, "-") {
				return cliConfig{}, fmt.Errorf("unknown flag %q", arg)
			}
			if cfg.output != "" {
				return cliConfig{}, fmt.Errorf("download accepts at most one output path")
			}
			cfg.output = arg
		}
	}
	return cfg, nil
}

func parseInspectCommand(args []string) (cliConfig, error) {
	cfg := cliConfig{command: "inspect"}
	for _, arg := range args {
		switch arg {
		case "--json":
			cfg.json = true
		case "-h", "--help":
			return cliConfig{command: "help"}, nil
		default:
			if strings.HasPrefix(arg, "-") {
				return cliConfig{}, fmt.Errorf("unknown flag %q", arg)
			}
			if cfg.path != "" {
				return cliConfig{}, errors.New("inspect accepts only one path")
			}
			cfg.path = arg
		}
	}
	if cfg.path == "" {
		return cliConfig{}, errors.New("inspect requires a path to a package file")
	}
	return cfg, nil
}

func runTUI() {
	prog := tea.NewProgram(newModel(defaultTargetSpec()), tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		exitErr(err)
	}
}

func runProbe(cfg cliConfig) {
	target, err := resolveTargetSpec(cfg.platform, cfg.arch)
	if err != nil {
		exitErr(err)
	}
	result, err := ProbeLatest(target)
	if err != nil {
		exitErr(err)
	}
	if cfg.json {
		emitJSON(os.Stdout, result)
		return
	}
	printProbeSummary(os.Stdout, result)
}

func runDownload(cfg cliConfig) {
	target, err := resolveTargetSpec(cfg.platform, cfg.arch)
	if err != nil {
		exitErr(err)
	}
	result, err := DownloadLatest(target, cfg.output)
	if err != nil {
		exitErr(err)
	}
	if cfg.json {
		emitJSON(os.Stdout, result)
		return
	}
	printDownloadSummary(os.Stdout, result)
}

func runInspect(path string, jsonOut bool) {
	result, err := InspectLocal(path)
	if err != nil {
		exitErr(err)
	}
	if jsonOut {
		emitJSON(os.Stdout, result)
		return
	}
	printInspectSummary(os.Stdout, result)
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "codex-unpacker v%s\n", appVersion)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  codex-unpacker")
	fmt.Fprintln(w, "  codex-unpacker probe [--platform windows|macos] [--arch x64|arm64] [--json]")
	fmt.Fprintln(w, "  codex-unpacker download [--platform windows|macos] [--arch x64|arm64] [--output <folder-or-file>] [--json]")
	fmt.Fprintln(w, "  codex-unpacker inspect <path.msix|path.dmg> [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "The default download target is your current platform Downloads folder.")
}

func printProbeSummary(w io.Writer, result ProbeResult) {
	fmt.Fprintln(w, "Codex Unpacker probe")
	fmt.Fprintf(w, "Target: %s\n", targetLabel(result.Target))
	fmt.Fprintf(w, "Latest version: %s\n", orText(result.Source.Version, "unknown"))
	if result.Source.SourceKind != "" {
		fmt.Fprintf(w, "Source: %s\n", result.Source.SourceKind)
	}
	if result.Source.PackageKind != "" {
		fmt.Fprintf(w, "Package: %s\n", packageKindLabel(result.Source.PackageKind))
	}
	if result.Source.AssetName != "" {
		fmt.Fprintf(w, "Artifact: %s\n", result.Source.AssetName)
	}
	if result.Source.DownloadURL != "" {
		fmt.Fprintf(w, "Download URL: %s\n", result.Source.DownloadURL)
	}
	if result.Source.ExpectedSHA256 != "" {
		fmt.Fprintf(w, "SHA256: %s\n", result.Source.ExpectedSHA256)
	}
	if result.State.Package.Version != "" {
		fmt.Fprintf(w, "Saved version: %s (%s)\n", result.State.Package.Version, shortHash(result.State.Package.SHA256))
	} else {
		fmt.Fprintln(w, "Saved version: none for this target")
	}
	fmt.Fprintf(w, "Default destination: %s\n", result.DefaultDestination)
	if result.WouldUpdate {
		fmt.Fprintln(w, "Status: update available")
	} else {
		fmt.Fprintln(w, "Status: already current")
	}
}

func printDownloadSummary(w io.Writer, result DownloadResult) {
	fmt.Fprintln(w, "Codex package downloaded")
	fmt.Fprintf(w, "Target: %s\n", targetLabel(result.Target))
	fmt.Fprintf(w, "Package: %s\n", packageKindLabel(result.Package.PackageKind))
	fmt.Fprintf(w, "Version: %s\n", result.Package.Version)
	fmt.Fprintf(w, "Saved to: %s\n", result.Destination)
	fmt.Fprintf(w, "SHA256: %s\n", result.Package.SHA256)
	fmt.Fprintf(w, "Size: %s\n", formatBytes(result.Package.Size))
}

func printInspectSummary(w io.Writer, result InspectResult) {
	fmt.Fprintln(w, "Codex package inspection")
	fmt.Fprintf(w, "Target: %s\n", targetLabel(result.Target))
	fmt.Fprintf(w, "Package: %s\n", packageKindLabel(result.Package.PackageKind))
	fmt.Fprintf(w, "Version: %s\n", result.Package.Version)
	fmt.Fprintf(w, "File: %s\n", result.Package.Path)
	fmt.Fprintf(w, "SHA256: %s\n", result.Package.SHA256)
	fmt.Fprintf(w, "Size: %s\n", formatBytes(result.Package.Size))
	if result.MatchesState {
		fmt.Fprintln(w, "Status: matches saved state")
	} else {
		fmt.Fprintln(w, "Status: validated locally")
	}
}

func emitJSON(w io.Writer, value any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func orText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
