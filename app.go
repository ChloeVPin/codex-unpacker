package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	appName             = "codex-unpacker"
	appVersion          = "1.0.3"
	packageName         = "OpenAI.Codex"
	packageArchitecture = "x64"
	packageFamilySuffix = "2p2nqsd0c76g0"
	packageExtension    = ".Msix"
	officialFeedURL     = "https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json"
	mirrorLatestURL     = "https://api.github.com/repos/Wangnov/codex-app-mirror/releases/latest"
	statePath           = "data/latest.json"
)

var httpClient = &http.Client{Timeout: 45 * time.Minute}

type StoredState struct {
	SchemaVersion int                          `json:"schemaVersion"`
	UpdatedAt     string                       `json:"updatedAt"`
	Targets       map[string]StoredTargetState `json:"targets,omitempty"`
}

type StoredTargetState struct {
	UpdatedAt string         `json:"updatedAt"`
	Package   PackageDetails `json:"package"`
	Source    ResolvedSource `json:"source,omitempty"`
}

type TargetSpec struct {
	Platform     string `json:"platform"`
	Architecture string `json:"architecture,omitempty"`
}

type PackageDetails struct {
	Target            TargetSpec `json:"target"`
	PackageKind       string     `json:"packageKind"`
	Name              string     `json:"name"`
	Version           string     `json:"version"`
	PackageMoniker    string     `json:"packageMoniker"`
	PackageFamilyName string     `json:"packageFamilyName"`
	Publisher         string     `json:"publisher"`
	Architecture      string     `json:"architecture"`
	SHA256            string     `json:"sha256"`
	Size              int64      `json:"size"`
	FileName          string     `json:"fileName"`
	Path              string     `json:"-"`
}

type ResolvedSource struct {
	Target           TargetSpec `json:"target"`
	SourceKind       string     `json:"sourceKind"`
	PackageKind      string     `json:"packageKind"`
	Version          string     `json:"version"`
	PackageIdentity  string     `json:"packageIdentity,omitempty"`
	StoreProductID   string     `json:"storeProductId,omitempty"`
	AssetName        string     `json:"assetName,omitempty"`
	Size             int64      `json:"size,omitempty"`
	DownloadURL      string     `json:"downloadUrl,omitempty"`
	ChecksumURL      string     `json:"checksumUrl,omitempty"`
	ExpectedSHA256   string     `json:"expectedSha256,omitempty"`
	AssetDigest      string     `json:"assetDigest,omitempty"`
	MirrorReleaseTag string     `json:"mirrorReleaseTag,omitempty"`
	MirrorReleaseURL string     `json:"mirrorReleaseUrl,omitempty"`
}

type ProbeResult struct {
	Target             TargetSpec        `json:"target"`
	State              StoredTargetState `json:"state"`
	Source             ResolvedSource    `json:"source"`
	DefaultDestination string            `json:"defaultDestination"`
	WouldUpdate        bool              `json:"wouldUpdate"`
}

type DownloadResult struct {
	Target      TargetSpec     `json:"target"`
	Package     PackageDetails `json:"package"`
	Source      ResolvedSource `json:"source"`
	Destination string         `json:"destination"`
}

type InspectResult struct {
	Target       TargetSpec     `json:"target"`
	Package      PackageDetails `json:"package"`
	MatchesState bool           `json:"matchesState"`
}

type officialManifest struct {
	SchemaVersion   int    `json:"schemaVersion"`
	BuildVersion    string `json:"buildVersion"`
	StoreProductID  string `json:"storeProductId"`
	PackageIdentity string `json:"packageIdentity"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	PackageURL      string `json:"packageUrl,omitempty"`
	MSIXURL         string `json:"msixUrl,omitempty"`
	URL             string `json:"url,omitempty"`
	AppInstallerURL string `json:"appInstallerUrl,omitempty"`
}

type mirrorRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []mirrorAsset `json:"assets"`
}

type mirrorAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest,omitempty"`
	Size               int64  `json:"size,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
}

type mirrorManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	GeneratedAt   string `json:"generatedAt"`
	Sources       struct {
		Windows struct {
			ProductID         string           `json:"productId"`
			Architecture      string           `json:"architecture"`
			Version           string           `json:"version"`
			PackageMoniker    string           `json:"packageMoniker"`
			URLHost           string           `json:"urlHost"`
			UpdateManifestURL string           `json:"updateManifestUrl"`
			UpdateManifest    officialManifest `json:"updateManifest"`
			ContentLength     int64            `json:"contentLength"`
			LastModified      string           `json:"lastModified"`
			ETag              string           `json:"etag"`
		} `json:"windows"`
		MacOS struct {
			Arm64 mirrorMacSource `json:"arm64"`
			X64   mirrorMacSource `json:"x64"`
		} `json:"macos"`
	} `json:"sources"`
}

type mirrorMacSource struct {
	URL                  string `json:"url"`
	AppcastURL           string `json:"appcastUrl"`
	ContentLength        int64  `json:"contentLength"`
	LastModified         string `json:"lastModified"`
	ETag                 string `json:"etag"`
	BundleShortVersion   string `json:"bundleShortVersion"`
	BundleVersion        string `json:"bundleVersion"`
	BundleIdentifier     string `json:"bundleIdentifier"`
	MinimumSystemVersion string `json:"minimumSystemVersion"`
	SHA256               string `json:"sha256"`
}

type appxManifest struct {
	Identity struct {
		Name                  string `xml:"Name,attr"`
		ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
		Version               string `xml:"Version,attr"`
		Publisher             string `xml:"Publisher,attr"`
	} `xml:"Identity"`
}

// LoadState returns the cached local state if it exists.
func LoadState() (StoredState, error) {
	body, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StoredState{}, nil
		}
		return StoredState{}, err
	}
	var envelope struct {
		SchemaVersion int                          `json:"schemaVersion"`
		UpdatedAt     string                       `json:"updatedAt"`
		Targets       map[string]StoredTargetState `json:"targets"`
		Package       PackageDetails               `json:"package"`
		Source        ResolvedSource               `json:"source,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return StoredState{}, err
	}

	state := StoredState{
		SchemaVersion: envelope.SchemaVersion,
		UpdatedAt:     envelope.UpdatedAt,
		Targets:       envelope.Targets,
	}
	if len(state.Targets) == 0 && envelope.Package.Version != "" {
		target := inferTargetSpecFromPackage(envelope.Package)
		envelope.Package.Target = target
		if envelope.Package.PackageKind == "" {
			switch target.Platform {
			case "macos":
				envelope.Package.PackageKind = "dmg"
			default:
				envelope.Package.PackageKind = "msix"
			}
		}
		envelope.Source.Target = target
		if envelope.Source.PackageKind == "" {
			envelope.Source.PackageKind = envelope.Package.PackageKind
		}
		state.Targets = map[string]StoredTargetState{
			targetKey(target): {
				UpdatedAt: envelope.UpdatedAt,
				Package:   envelope.Package,
				Source:    envelope.Source,
			},
		}
	}
	if state.Targets == nil {
		state.Targets = map[string]StoredTargetState{}
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	return state, nil
}

func SaveState(state StoredState) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 2
	}
	if state.Targets == nil {
		state.Targets = map[string]StoredTargetState{}
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(statePath, body, 0o644)
}

func defaultTargetSpec() TargetSpec {
	switch runtime.GOOS {
	case "windows":
		return TargetSpec{Platform: "windows", Architecture: "x64"}
	case "darwin", "linux":
		return TargetSpec{Platform: "macos", Architecture: architectureFromGo(runtime.GOARCH)}
	default:
		return TargetSpec{Platform: strings.ToLower(runtime.GOOS), Architecture: architectureFromGo(runtime.GOARCH)}
	}
}

func resolveTargetSpec(platform, architecture string) (TargetSpec, error) {
	spec := defaultTargetSpec()
	platformProvided := strings.TrimSpace(platform) != ""
	archProvided := strings.TrimSpace(architecture) != ""

	if platformProvided {
		spec.Platform = normalizePlatform(platform)
		if !archProvided {
			spec.Architecture = ""
		}
	}
	if archProvided {
		spec.Architecture = normalizeArchitecture(architecture)
	}

	switch spec.Platform {
	case "windows":
		if spec.Architecture == "" {
			spec.Architecture = "x64"
		}
		if spec.Architecture != "x64" {
			return TargetSpec{}, fmt.Errorf("windows downloads only support x64 at the moment")
		}
	case "macos":
		if spec.Architecture == "" {
			if spec.Platform == "macos" && runtime.GOOS != "darwin" {
				spec.Architecture = "arm64"
			} else {
				spec.Architecture = architectureFromGo(runtime.GOARCH)
			}
		}
		if spec.Architecture != "arm64" && spec.Architecture != "x64" {
			return TargetSpec{}, fmt.Errorf("macOS downloads only support arm64 or x64")
		}
	default:
		return TargetSpec{}, fmt.Errorf("unsupported platform %q", spec.Platform)
	}

	return spec, nil
}

func ProbeLatest(target TargetSpec) (ProbeResult, error) {
	state, err := LoadState()
	if err != nil {
		return ProbeResult{}, err
	}
	source, err := resolveLatestSource(target)
	if err != nil {
		return ProbeResult{}, err
	}

	saved := stateForTarget(state, target)
	wouldUpdate := !packageMatchesSource(saved.Package, source)

	destination, err := defaultDownloadPath(source.AssetName)
	if err != nil {
		return ProbeResult{}, err
	}

	return ProbeResult{
		Target:             target,
		State:              saved,
		Source:             source,
		DefaultDestination: destination,
		WouldUpdate:        wouldUpdate,
	}, nil
}

func DownloadLatest(target TargetSpec, output string) (DownloadResult, error) {
	source, err := resolveLatestSource(target)
	if err != nil {
		return DownloadResult{}, err
	}

	destination, err := resolveTargetPath(output, source.AssetName)
	if err != nil {
		return DownloadResult{}, err
	}

	pkg, err := downloadAndInspect(source, destination)
	if err != nil {
		return DownloadResult{}, err
	}

	state, err := LoadState()
	if err != nil {
		return DownloadResult{}, err
	}
	if state.Targets == nil {
		state.Targets = map[string]StoredTargetState{}
	}
	state.Targets[targetKey(target)] = StoredTargetState{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Package:   pkg,
		Source:    source,
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := SaveState(state); err != nil {
		return DownloadResult{}, err
	}

	return DownloadResult{
		Target:      target,
		Package:     pkg,
		Source:      source,
		Destination: destination,
	}, nil
}

func InspectLocal(path string) (InspectResult, error) {
	pkg, err := inspectLocalPackage(path)
	if err != nil {
		return InspectResult{}, err
	}
	state, err := LoadState()
	if err != nil {
		return InspectResult{}, err
	}

	saved := stateForTarget(state, pkg.Target)
	matches := packageMatchesPackage(saved.Package, pkg)

	return InspectResult{
		Target:       pkg.Target,
		Package:      pkg,
		MatchesState: matches,
	}, nil
}

func resolveLatestSource(target TargetSpec) (ResolvedSource, error) {
	switch target.Platform {
	case "windows":
		return resolveWindowsSource(target)
	case "macos":
		return resolveMacOSSource(target)
	default:
		return ResolvedSource{}, fmt.Errorf("unsupported platform %q", target.Platform)
	}
}

func resolveWindowsSource(target TargetSpec) (ResolvedSource, error) {
	manifest, err := fetchOfficialManifest()
	if err != nil {
		return ResolvedSource{}, err
	}
	if manifest.PackageIdentity != "" && !strings.EqualFold(manifest.PackageIdentity, packageName) {
		return ResolvedSource{}, fmt.Errorf("unexpected package identity %q", manifest.PackageIdentity)
	}

	version := strings.TrimSpace(manifest.BuildVersion)
	packageIdentity := strings.TrimSpace(manifest.PackageIdentity)
	storeProductID := strings.TrimSpace(manifest.StoreProductID)

	if direct := directPackageURLFromManifest(manifest); direct != "" {
		name := filepath.Base(direct)
		if version == "" {
			version = versionFromAssetName(name)
		}
		if version == "" {
			return ResolvedSource{}, fmt.Errorf("official manifest points to %q but the version could not be determined", direct)
		}
		return ResolvedSource{
			Target:          target,
			SourceKind:      "OfficialManifest",
			PackageKind:     "msix",
			Version:         version,
			PackageIdentity: packageIdentity,
			StoreProductID:  storeProductID,
			AssetName:       name,
			DownloadURL:     direct,
		}, nil
	}

	release, err := fetchMirrorRelease()
	if err != nil {
		return ResolvedSource{}, err
	}

	asset, err := findMirrorAsset(release.Assets, version)
	if err != nil {
		return ResolvedSource{}, err
	}
	if version == "" {
		version = versionFromAssetName(asset.Name)
	}
	if version == "" {
		return ResolvedSource{}, fmt.Errorf("unable to infer Codex version from %q", asset.Name)
	}

	checksumURL, checksum := findWindowsChecksum(release.Assets, asset.Name)
	if checksum == "" {
		checksum = parseDigest(asset.Digest)
	}

	return ResolvedSource{
		Target:           target,
		SourceKind:       "MirrorRelease",
		PackageKind:      "msix",
		Version:          version,
		PackageIdentity:  packageIdentity,
		StoreProductID:   storeProductID,
		AssetName:        asset.Name,
		Size:             asset.Size,
		DownloadURL:      asset.BrowserDownloadURL,
		ChecksumURL:      checksumURL,
		ExpectedSHA256:   checksum,
		AssetDigest:      parseDigest(asset.Digest),
		MirrorReleaseTag: release.TagName,
		MirrorReleaseURL: release.HTMLURL,
	}, nil
}

func resolveMacOSSource(target TargetSpec) (ResolvedSource, error) {
	release, err := fetchMirrorRelease()
	if err != nil {
		return ResolvedSource{}, err
	}

	manifest, err := fetchMirrorManifest(release.Assets)
	if err != nil {
		return ResolvedSource{}, err
	}

	macSource, err := selectMacSource(manifest, target.Architecture)
	if err != nil {
		return ResolvedSource{}, err
	}

	version := strings.TrimSpace(macSource.BundleShortVersion)
	if version == "" {
		version = versionFromAssetName(macSource.URL)
	}
	if version == "" {
		return ResolvedSource{}, fmt.Errorf("unable to infer macOS version from %q", macSource.URL)
	}

	assetName := filepath.Base(macSource.URL)
	expectedSHA256 := strings.TrimSpace(macSource.SHA256)
	if expectedSHA256 == "" {
		return ResolvedSource{}, fmt.Errorf("mirror manifest is missing the macOS SHA256 for %s", target.Architecture)
	}

	return ResolvedSource{
		Target:           target,
		SourceKind:       "MirrorManifest",
		PackageKind:      "dmg",
		Version:          version,
		AssetName:        assetName,
		Size:             macSource.ContentLength,
		DownloadURL:      macSource.URL,
		ChecksumURL:      releaseAssetURL(release.Assets, "SHA256SUMS-macos.txt"),
		ExpectedSHA256:   expectedSHA256,
		AssetDigest:      expectedSHA256,
		MirrorReleaseTag: release.TagName,
		MirrorReleaseURL: release.HTMLURL,
	}, nil
}

func fetchOfficialManifest() (officialManifest, error) {
	var manifest officialManifest
	if err := fetchJSON(officialFeedURL, &manifest); err != nil {
		return officialManifest{}, fmt.Errorf("official manifest fetch failed: %w", err)
	}
	return manifest, nil
}

func fetchMirrorRelease() (mirrorRelease, error) {
	var release mirrorRelease
	if err := fetchJSON(mirrorLatestURL, &release); err != nil {
		return mirrorRelease{}, fmt.Errorf("mirror release fetch failed: %w", err)
	}
	return release, nil
}

func fetchMirrorManifest(assets []mirrorAsset) (mirrorManifest, error) {
	assetURL := releaseAssetURL(assets, "release-manifest.json")
	if assetURL == "" {
		return mirrorManifest{}, errors.New("mirror release is missing release-manifest.json")
	}

	var manifest mirrorManifest
	if err := fetchJSON(assetURL, &manifest); err != nil {
		return mirrorManifest{}, fmt.Errorf("mirror manifest fetch failed: %w", err)
	}
	return manifest, nil
}

func releaseAssetURL(assets []mirrorAsset, name string) string {
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, name) {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func selectMacSource(manifest mirrorManifest, architecture string) (mirrorMacSource, error) {
	switch normalizeArchitecture(architecture) {
	case "", "arm64":
		if strings.TrimSpace(manifest.Sources.MacOS.Arm64.URL) != "" {
			return manifest.Sources.MacOS.Arm64, nil
		}
		if strings.TrimSpace(manifest.Sources.MacOS.X64.URL) != "" {
			return manifest.Sources.MacOS.X64, nil
		}
	case "x64":
		if strings.TrimSpace(manifest.Sources.MacOS.X64.URL) != "" {
			return manifest.Sources.MacOS.X64, nil
		}
		if strings.TrimSpace(manifest.Sources.MacOS.Arm64.URL) != "" {
			return manifest.Sources.MacOS.Arm64, nil
		}
	default:
		return mirrorMacSource{}, fmt.Errorf("unsupported macOS architecture %q", architecture)
	}

	return mirrorMacSource{}, errors.New("mirror manifest is missing macOS download metadata")
}

func directPackageURLFromManifest(manifest officialManifest) string {
	for _, candidate := range []string{
		manifest.DownloadURL,
		manifest.PackageURL,
		manifest.MSIXURL,
		manifest.URL,
	} {
		if looksLikePackageURL(candidate) {
			return candidate
		}
	}
	return ""
}

func looksLikePackageURL(candidate string) bool {
	lower := strings.ToLower(strings.TrimSpace(candidate))
	switch {
	case strings.HasSuffix(lower, ".msix"):
		return true
	case strings.HasSuffix(lower, ".appx"):
		return true
	case strings.HasSuffix(lower, ".msixbundle"):
		return true
	case strings.HasSuffix(lower, ".appxbundle"):
		return true
	default:
		return false
	}
}

func findMirrorAsset(assets []mirrorAsset, version string) (mirrorAsset, error) {
	if version != "" {
		want := packageAssetName(version)
		for _, asset := range assets {
			if strings.EqualFold(asset.Name, want) {
				return asset, nil
			}
		}
	}

	for _, asset := range assets {
		if versionFromAssetName(asset.Name) != "" && strings.EqualFold(filepath.Ext(asset.Name), packageExtension) {
			return asset, nil
		}
	}

	want := packageAssetName(version)
	if version == "" {
		want = packageName + "_" + packageArchitecture + "__" + packageFamilySuffix + packageExtension
	}
	return mirrorAsset{}, fmt.Errorf("mirror release is missing the Windows MSIX asset %q", want)
}

func findWindowsChecksum(assets []mirrorAsset, fileName string) (string, string) {
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, "SHA256SUMS-windows.txt") {
			body, err := fetchText(asset.BrowserDownloadURL)
			if err != nil {
				return asset.BrowserDownloadURL, ""
			}
			if hash := parseChecksum(body, fileName); hash != "" {
				return asset.BrowserDownloadURL, hash
			}
			return asset.BrowserDownloadURL, ""
		}
	}
	return "", ""
}

func downloadAndInspect(source ResolvedSource, destination string) (PackageDetails, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return PackageDetails{}, err
	}

	tempDir, err := os.MkdirTemp("", "codex-unpacker-*")
	if err != nil {
		return PackageDetails{}, err
	}
	defer os.RemoveAll(tempDir)

	tempPath := filepath.Join(tempDir, filepath.Base(destination))
	if err := downloadFile(source.DownloadURL, tempPath); err != nil {
		return PackageDetails{}, err
	}

	pkg, err := inspectDownloadedPackage(tempPath, source)
	if err != nil {
		return PackageDetails{}, err
	}

	expectedHash := source.ExpectedSHA256
	if expectedHash == "" {
		expectedHash = parseDigest(source.AssetDigest)
	}
	if expectedHash != "" && !sameHash(pkg.SHA256, expectedHash) {
		return PackageDetails{}, fmt.Errorf("downloaded package hash mismatch: expected %s, got %s", expectedHash, pkg.SHA256)
	}

	if err := copyFile(tempPath, destination); err != nil {
		return PackageDetails{}, err
	}
	pkg.Path = destination
	pkg.FileName = filepath.Base(destination)
	return pkg, nil
}

func inspectDownloadedPackage(path string, source ResolvedSource) (PackageDetails, error) {
	switch strings.ToLower(strings.TrimSpace(source.PackageKind)) {
	case "msix":
		return inspectMSIX(path, source.Version, source.Target)
	case "dmg":
		return inspectDMG(path, source)
	default:
		return PackageDetails{}, fmt.Errorf("unsupported package kind %q", source.PackageKind)
	}
}

func inspectLocalPackage(path string) (PackageDetails, error) {
	target := inferTargetSpecFromPath(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".msix", ".appx", ".msixbundle", ".appxbundle":
		pkg, err := inspectMSIX(path, "", target)
		if err != nil {
			return PackageDetails{}, err
		}
		pkg.Target = target
		pkg.PackageKind = "msix"
		return pkg, nil
	case ".dmg":
		pkg, err := inspectDMG(path, ResolvedSource{Target: target, PackageKind: "dmg"})
		if err != nil {
			return PackageDetails{}, err
		}
		return pkg, nil
	default:
		return PackageDetails{}, fmt.Errorf("unsupported package type %q", filepath.Ext(path))
	}
}

func inspectDMG(path string, source ResolvedSource) (PackageDetails, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return PackageDetails{}, err
	}

	sum, size, err := hashFile(abs)
	if err != nil {
		return PackageDetails{}, err
	}
	if expected := source.ExpectedSHA256; expected != "" && !sameHash(sum, expected) {
		return PackageDetails{}, fmt.Errorf("downloaded package hash mismatch: expected %s, got %s", expected, sum)
	}

	fileName := filepath.Base(abs)
	version := source.Version
	if version == "" {
		version = versionFromAssetName(fileName)
	}

	return PackageDetails{
		Target:       source.Target,
		PackageKind:  "dmg",
		Name:         packageName,
		Version:      version,
		SHA256:       sum,
		Size:         size,
		FileName:     fileName,
		Path:         abs,
		Architecture: source.Target.Architecture,
	}, nil
}

func inspectMSIX(path string, expectedVersion string, target TargetSpec) (PackageDetails, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return PackageDetails{}, err
	}

	reader, err := zip.OpenReader(abs)
	if err != nil {
		return PackageDetails{}, err
	}
	defer reader.Close()

	required := map[string]bool{
		"AppxManifest.xml":               false,
		"AppxBlockMap.xml":               false,
		"AppxSignature.p7x":              false,
		"AppxMetadata/CodeIntegrity.cat": false,
	}

	var manifestBytes []byte
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if _, ok := required[name]; ok {
			required[name] = true
		}
		if name == "AppxManifest.xml" {
			rc, err := file.Open()
			if err != nil {
				return PackageDetails{}, err
			}
			manifestBytes, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return PackageDetails{}, err
			}
		}
	}

	for name, seen := range required {
		if !seen {
			return PackageDetails{}, fmt.Errorf("MSIX is missing %s", name)
		}
	}
	if len(manifestBytes) == 0 {
		return PackageDetails{}, errors.New("MSIX is missing AppxManifest.xml")
	}

	var manifest appxManifest
	if err := xml.Unmarshal(manifestBytes, &manifest); err != nil {
		return PackageDetails{}, err
	}
	if !strings.EqualFold(manifest.Identity.Name, packageName) {
		return PackageDetails{}, fmt.Errorf("unexpected package identity %s", manifest.Identity.Name)
	}
	if manifest.Identity.ProcessorArchitecture != "" && !strings.EqualFold(manifest.Identity.ProcessorArchitecture, packageArchitecture) {
		return PackageDetails{}, fmt.Errorf("unexpected architecture %s", manifest.Identity.ProcessorArchitecture)
	}
	if expectedVersion != "" && manifest.Identity.Version != expectedVersion {
		return PackageDetails{}, fmt.Errorf("expected version %s, got %s", expectedVersion, manifest.Identity.Version)
	}

	sum, size, err := hashFile(abs)
	if err != nil {
		return PackageDetails{}, err
	}

	fileName := filepath.Base(abs)
	moniker := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if version := versionFromAssetName(fileName); version != "" && version != manifest.Identity.Version {
		return PackageDetails{}, fmt.Errorf("filename version %s does not match manifest version %s", version, manifest.Identity.Version)
	}

	return PackageDetails{
		Target:            target,
		PackageKind:       "msix",
		Name:              manifest.Identity.Name,
		Version:           manifest.Identity.Version,
		PackageMoniker:    moniker,
		PackageFamilyName: packageName + "_" + packageFamilySuffix,
		Publisher:         manifest.Identity.Publisher,
		Architecture:      manifest.Identity.ProcessorArchitecture,
		SHA256:            sum,
		Size:              size,
		FileName:          fileName,
		Path:              abs,
	}, nil
}

func defaultDownloadPath(fileName string) (string, error) {
	downloads, err := downloadsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(downloads, resolvedFileName(fileName)), nil
}

func downloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

func resolveTargetPath(target string, defaultName string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return defaultDownloadPath(defaultName)
	}

	cleaned := filepath.Clean(target)
	if strings.HasSuffix(target, string(os.PathSeparator)) || strings.HasSuffix(target, "/") || strings.HasSuffix(target, `\`) {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	if filepath.Ext(filepath.Base(cleaned)) == "" {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	return cleaned, nil
}

func resolvedFileName(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return appName
	}
	return filepath.Base(candidate)
}

func packageAssetName(version string) string {
	if version == "" {
		return packageName + "_latest_" + packageArchitecture + "__" + packageFamilySuffix + packageExtension
	}
	return fmt.Sprintf("%s_%s_%s__%s%s", packageName, version, packageArchitecture, packageFamilySuffix, packageExtension)
}

func versionFromAssetName(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	switch {
	case strings.HasPrefix(base, packageName+"_") && strings.Contains(base, "__"+packageFamilySuffix):
		prefix := packageName + "_"
		suffix := "_" + packageArchitecture + "__" + packageFamilySuffix
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(strings.TrimPrefix(base, prefix), suffix)
		}
	case strings.HasPrefix(base, "Codex-"):
		tail := strings.TrimPrefix(base, "Codex-")
		parts := strings.Split(tail, "-")
		if len(parts) >= 2 {
			last := strings.ToLower(parts[len(parts)-1])
			if isArchitectureToken(last) {
				version := strings.Join(parts[:len(parts)-1], "-")
				if looksLikeVersion(version) {
					return version
				}
			}
		}
		if len(parts) >= 3 && strings.EqualFold(parts[0], "darwin") {
			mid := strings.ToLower(parts[1])
			if isArchitectureToken(mid) {
				version := strings.Join(parts[2:], "-")
				if looksLikeVersion(version) {
					return version
				}
			}
		}
	}
	return ""
}

func inferTargetSpecFromPackage(pkg PackageDetails) TargetSpec {
	if pkg.Target.Platform != "" {
		return TargetSpec{
			Platform:     normalizePlatform(pkg.Target.Platform),
			Architecture: normalizeArchitecture(pkg.Target.Architecture),
		}
	}
	if strings.EqualFold(pkg.PackageKind, "dmg") || strings.EqualFold(filepath.Ext(pkg.FileName), ".dmg") {
		return TargetSpec{
			Platform:     "macos",
			Architecture: normalizeArchitecture(pkg.Architecture),
		}
	}
	arch := normalizeArchitecture(pkg.Architecture)
	if arch == "" {
		arch = "x64"
	}
	return TargetSpec{
		Platform:     "windows",
		Architecture: arch,
	}
}

func inferTargetSpecFromPath(path string) TargetSpec {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".msix", ".appx", ".msixbundle", ".appxbundle":
		return TargetSpec{Platform: "windows", Architecture: "x64"}
	case ".dmg":
		arch := ""
		lower := strings.ToLower(base)
		switch {
		case strings.Contains(lower, "arm64"):
			arch = "arm64"
		case strings.Contains(lower, "x64"), strings.Contains(lower, "amd64"):
			arch = "x64"
		}
		return TargetSpec{Platform: "macos", Architecture: arch}
	default:
		return TargetSpec{}
	}
}

func stateForTarget(state StoredState, target TargetSpec) StoredTargetState {
	if len(state.Targets) == 0 {
		return StoredTargetState{}
	}
	if entry, ok := state.Targets[targetKey(target)]; ok {
		return entry
	}
	return StoredTargetState{}
}

func packageMatchesSource(pkg PackageDetails, source ResolvedSource) bool {
	if pkg.Version == "" || source.Version == "" {
		return false
	}
	if !strings.EqualFold(pkg.Version, source.Version) {
		return false
	}
	if source.ExpectedSHA256 != "" && !sameHash(pkg.SHA256, source.ExpectedSHA256) {
		return false
	}
	if source.PackageKind != "" && !strings.EqualFold(pkg.PackageKind, source.PackageKind) {
		return false
	}
	return true
}

func packageMatchesPackage(a, b PackageDetails) bool {
	if a.Version == "" || b.Version == "" {
		return false
	}
	if !strings.EqualFold(a.Version, b.Version) {
		return false
	}
	if !sameHash(a.SHA256, b.SHA256) {
		return false
	}
	if a.PackageKind != "" && b.PackageKind != "" && !strings.EqualFold(a.PackageKind, b.PackageKind) {
		return false
	}
	return true
}

func targetKey(target TargetSpec) string {
	return strings.ToLower(strings.TrimSpace(target.Platform)) + "/" + strings.ToLower(strings.TrimSpace(target.Architecture))
}

func targetLabel(target TargetSpec) string {
	platform := strings.TrimSpace(target.Platform)
	switch strings.ToLower(platform) {
	case "windows":
		platform = "Windows"
	case "macos":
		platform = "macOS"
	default:
		platform = titleCase(platform)
	}
	arch := strings.TrimSpace(target.Architecture)
	if arch == "" {
		return platform
	}
	return platform + " " + arch
}

func packageKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "msix":
		return "MSIX"
	case "dmg":
		return "DMG"
	default:
		return strings.ToUpper(strings.TrimSpace(kind))
	}
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(value[:1]) + strings.ToLower(value[1:])
}

func normalizePlatform(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "windows", "win", "win32":
		return "windows"
	case "macos", "darwin", "mac", "linux":
		return "macos"
	case "auto":
		return defaultTargetSpec().Platform
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return ""
	case "amd64", "x86_64":
		return "x64"
	case "x64":
		return "x64"
	case "arm64", "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func architectureFromGo(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return normalizeArchitecture(value)
	}
}

func isArchitectureToken(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x64", "arm64", "amd64", "x86_64", "aarch64":
		return true
	default:
		return false
	}
}

func looksLikeVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Count(value, ".") >= 1 {
		return true
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseChecksum(body, fileName string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		name := strings.TrimSpace(fields[len(fields)-1])
		name = strings.TrimPrefix(name, "*")
		if len(hash) == 64 && strings.EqualFold(name, fileName) {
			return hash
		}
	}
	return ""
}

func parseDigest(digest string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(digest)), "sha256:")
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func fetchJSON(url string, out any) error {
	body, err := fetchBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func fetchText(url string) (string, error) {
	body, err := fetchBytes(url)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func fetchBytes(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", appName+"/"+appVersion)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(snippet)))
	}
	return io.ReadAll(resp.Body)
}

func downloadFile(url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", appName+"/"+appVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(snippet)))
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// Sync flushes OS write buffers; the deferred Close handles the fd cleanup.
	return out.Sync()
}

func sameHash(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}
